package invoicefile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LocalStorage 将附件写入配置的根目录下。
// 文件按年月分子目录组织：<root>/<YYYY>/<MM>/<storedName>。
// key 即相对路径；所有写入/读取都以 root 为 jail，写入前会重新 Clean 并校验前缀。
type LocalStorage struct {
	root string
}

// NewLocalStorage 以绝对化后的 root 作为 jail。root 为空时回退到默认路径。
// 会尝试创建根目录，失败直接返回错误（避免运行时写入时才发现权限问题）。
func NewLocalStorage(root string) (*LocalStorage, error) {
	if strings.TrimSpace(root) == "" {
		root = "/data/invoice_files"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &LocalStorage{root: abs}, nil
}

func (s *LocalStorage) Kind() string { return "local" }

// safePath 解析并校验 key，防止 ".." / 绝对路径越狱。
// 只接受"干净的相对路径"：不含 ".."、不以 "/" 开头、不含 "\\" 等分隔符变体。
// 返回的路径保证位于 root 下。
func (s *LocalStorage) safePath(key string) (string, error) {
	if key == "" {
		return "", errors.New("empty storage key")
	}
	// 硬性拒绝常见越狱模式，避免任何 Clean 后"看似正常"的绕过。
	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, string(filepath.Separator)) {
		return "", errors.New("invalid storage key: absolute path")
	}
	if key == ".." || strings.HasPrefix(key, "../") || strings.Contains(key, "/../") || strings.HasSuffix(key, "/..") {
		return "", errors.New("invalid storage key: traversal")
	}
	if strings.Contains(key, "\\") {
		return "", errors.New("invalid storage key: backslash")
	}
	// Clean 后不允许出现变化（说明原本就不干净）。
	// Storage keys are portable, slash-separated object keys. path.Clean keeps
	// their normalization stable on Windows while filepath.FromSlash maps them
	// only at the final filesystem boundary.
	cleaned := path.Clean(key)
	if cleaned != key || cleaned == "." || cleaned == "/" {
		return "", errors.New("invalid storage key: not normalized")
	}
	if filepath.IsAbs(cleaned) {
		return "", errors.New("invalid storage key: absolute after clean")
	}
	full := filepath.Join(s.root, filepath.FromSlash(cleaned))
	// 再次确认 full 在 root 下（防御 Windows 分隔符与 symlink 异常）。
	rel, err := filepath.Rel(s.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("invalid storage key: outside root")
	}
	return full, nil
}

func (s *LocalStorage) Put(ctx context.Context, key string, r io.Reader, size int64, _ string) error {
	full, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return err
	}
	// O_EXCL 防止同 key 覆盖（UUID 冲突概率可忽略，但有就该拒绝而非吞掉）。
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	if size > 0 {
		// 用 LimitReader 防御对端声明与实际写入不一致；Copy 失败时清理半截文件。
		_, err = io.Copy(f, io.LimitReader(r, size))
	} else {
		_, err = io.Copy(f, r)
	}
	if err != nil {
		_ = os.Remove(full)
		return err
	}
	return nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	full, err := s.safePath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	full, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	full, err := s.safePath(key)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(full); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *LocalStorage) ListKeysPage(ctx context.Context, prefix string, cursor string, limit int) ([]string, string, error) {
	if limit <= 0 {
		limit = 2000
	}
	prefix = strings.TrimPrefix(path.Clean("/"+prefix), "/")
	if prefix == "." {
		prefix = ""
	}
	allKeys := make([]string, 0, min(limit, 128))
	err := filepath.WalkDir(s.root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, filePath)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		allKeys = append(allKeys, key)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	sort.Strings(allKeys)
	start := 0
	if cursor != "" {
		start = sort.SearchStrings(allKeys, cursor)
		for start < len(allKeys) && allKeys[start] <= cursor {
			start++
		}
	}
	end := min(start+limit, len(allKeys))
	nextCursor := ""
	if end < len(allKeys) && end > start {
		nextCursor = allKeys[end-1]
	}
	return allKeys[start:end], nextCursor, nil
}

func (s *LocalStorage) SignedURL(ctx context.Context, key string, ttl time.Duration, filename string, inline bool) (string, error) {
	return "", ErrSignedURLNotSupported
}

// AbsolutePath 返回 key 对应的本地绝对路径，给下载 controller 用 http.ServeFile。
func (s *LocalStorage) AbsolutePath(key string) (string, error) {
	return s.safePath(key)
}
