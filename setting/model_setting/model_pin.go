package model_setting

import (
	"slices"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ModelPinTop    = "top"
	ModelPinBottom = "bottom"
)

// ModelPinSetting lists models an administrator moved to the top or bottom of
// every group they belong to. Pins apply when a single group is listed; lists
// across all groups keep the regular order. PinTop is ordered first to last;
// the most recently pinned model is placed first. PinBottom is ordered first
// to last; the most recently pinned model is placed last.
type ModelPinSetting struct {
	PinTop    []string `json:"pin_top"`
	PinBottom []string `json:"pin_bottom"`
}

var modelPinSetting = ModelPinSetting{
	PinTop:    []string{},
	PinBottom: []string{},
}

func init() {
	config.GlobalConfig.Register("model_pin_setting", &modelPinSetting)
}

// GetModelPin returns ModelPinTop, ModelPinBottom or "" for a model name.
func GetModelPin(name string) string {
	if slices.Contains(modelPinSetting.PinTop, name) {
		return ModelPinTop
	}
	if slices.Contains(modelPinSetting.PinBottom, name) {
		return ModelPinBottom
	}
	return ""
}

// PinnedModelLists returns new pin lists with name moved to position
// (ModelPinTop, ModelPinBottom, or "" to unpin). The current lists are not
// modified.
func PinnedModelLists(name string, position string) (top []string, bottom []string) {
	top = make([]string, 0, len(modelPinSetting.PinTop)+1)
	bottom = make([]string, 0, len(modelPinSetting.PinBottom)+1)
	if position == ModelPinTop {
		top = append(top, name)
	}
	for _, item := range modelPinSetting.PinTop {
		if item != name && !slices.Contains(top, item) {
			top = append(top, item)
		}
	}
	for _, item := range modelPinSetting.PinBottom {
		if item != name && !slices.Contains(bottom, item) {
			bottom = append(bottom, item)
		}
	}
	if position == ModelPinBottom {
		bottom = append(bottom, name)
	}
	return top, bottom
}

// GetModelPinsIn returns the pinned models present in models, keeping the
// configured order so models the reader cannot see are never exposed.
func GetModelPinsIn(models map[string]struct{}) (top []string, bottom []string) {
	top = make([]string, 0)
	bottom = make([]string, 0)
	for _, name := range modelPinSetting.PinTop {
		if _, ok := models[name]; ok && !slices.Contains(top, name) {
			top = append(top, name)
		}
	}
	for _, name := range modelPinSetting.PinBottom {
		if _, ok := models[name]; ok && !slices.Contains(top, name) && !slices.Contains(bottom, name) {
			bottom = append(bottom, name)
		}
	}
	return top, bottom
}

// OrderModelsPinned moves pinned-to-top models first and pinned-to-bottom
// models last, in the configured order. Other models keep their order.
func OrderModelsPinned(models []string) []string {
	present := make(map[string]struct{}, len(models))
	for _, name := range models {
		present[name] = struct{}{}
	}
	top, bottom := GetModelPinsIn(present)
	if len(top) == 0 && len(bottom) == 0 {
		return models
	}
	ordered := make([]string, 0, len(models))
	ordered = append(ordered, top...)
	for _, name := range models {
		if !slices.Contains(top, name) && !slices.Contains(bottom, name) {
			ordered = append(ordered, name)
		}
	}
	return append(ordered, bottom...)
}
