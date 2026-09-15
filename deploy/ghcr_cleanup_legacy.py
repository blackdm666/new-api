"""One-time, explicitly dispatched retirement of legacy GHCR image versions."""
import hashlib
import json
import os
from pathlib import Path
import re
import urllib.request

repo = os.environ['GITHUB_REPOSITORY'].lower()
owner, package = repo.split('/')
tag = os.environ['REPLACEMENT_TAG']
digest = os.environ['REPLACEMENT_DIGEST']
assert re.fullmatch(r'88api-[a-f0-9]{7,40}', tag)
assert re.fullmatch(r'sha256:[a-f0-9]{64}', digest)
headers = {'Authorization': 'Bearer ' + os.environ['GH_TOKEN'], 'Accept': 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28'}

def api(path, method='GET'):
    with urllib.request.urlopen(urllib.request.Request('https://api.github.com/' + path, headers=headers, method=method)) as response:
        raw = response.read()
        return json.loads(raw) if raw else None

with urllib.request.urlopen('https://ghcr.io/token?scope=repository:' + repo + ':pull') as response:
    pull_token = json.load(response)['token']
for replacement in (tag, '88api'):
    request = urllib.request.Request('https://ghcr.io/v2/' + repo + '/manifests/' + replacement,
        headers={'Authorization': 'Bearer ' + pull_token, 'Accept': 'application/vnd.oci.image.index.v1+json'})
    with urllib.request.urlopen(request) as response:
        assert response.headers['Docker-Content-Digest'] == digest
        assert 'sha256:' + hashlib.sha256(response.read()).hexdigest() == digest

base = f'users/{owner}/packages/container/{package}/versions'
versions = []
page = 1
while True:
    batch = api(f'{base}?per_page=100&page={page}')
    versions.extend(batch)
    if len(batch) < 100:
        break
    page += 1

def legacy(value):
    return value == 'seedance-2.5' or value.startswith('seedance-2.5-')

targets = []
for version in versions:
    tags = version['metadata']['container']['tags']
    if not any(legacy(t) for t in tags):
        continue
    assert tags and all(legacy(t) for t in tags), 'Legacy version shares a retained tag'
    assert version['name'] != digest, 'Replacement must be a separate manifest version'
    targets.append({'id': version['id'], 'digest': version['name'], 'tags': tags})
assert any(v['name'] == digest and tag in v['metadata']['container']['tags'] for v in versions)
Path('ghcr-cleanup-plan.json').write_text(json.dumps({'replacement': tag, 'digest': digest, 'targets': targets}, indent=2))
deleted = []
for target in targets:
    current = api(f"{base}/{target['id']}")
    assert current['name'] == target['digest']
    assert current['metadata']['container']['tags'] and all(legacy(t) for t in current['metadata']['container']['tags'])
    api(f"{base}/{target['id']}", method='DELETE')
    deleted.append(target)
    Path('ghcr-cleanup-deleted.json').write_text(json.dumps(deleted, indent=2))
    print(json.dumps(target), flush=True)
print(json.dumps({'deleted_versions': len(deleted), 'replacement': tag, 'untagged_and_other_versions_preserved': True}))
