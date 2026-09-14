package common

import (
	"errors"
	"fmt"
)

// ResolveMappedModelName resolves a channel model mapping without mutating
// relay state. All relay formats and capability discovery paths should share
// this function so chain and cycle behavior remains consistent.
func ResolveMappedModelName(originModel, modelMapping string, fallbackName ...func(string) string) (string, bool, error) {
	if modelMapping == "" || modelMapping == "{}" {
		return originModel, false, nil
	}
	modelMap := make(map[string]string)
	if err := Unmarshal([]byte(modelMapping), &modelMap); err != nil {
		return originModel, false, fmt.Errorf("unmarshal_model_mapping_failed")
	}

	currentModel := originModel
	visitedModels := map[string]bool{currentModel: true}
	isMapped := false
	for {
		mappedModel, exists := modelMap[currentModel]
		if (!exists || mappedModel == "") && len(fallbackName) > 0 && fallbackName[0] != nil {
			baseModel := fallbackName[0](currentModel)
			if baseModel != currentModel {
				mappedModel, exists = modelMap[baseModel]
			}
		}
		if !exists || mappedModel == "" {
			break
		}
		if visitedModels[mappedModel] {
			if mappedModel == currentModel {
				if currentModel == originModel {
					return originModel, false, nil
				}
				return currentModel, true, nil
			}
			return originModel, false, errors.New("model_mapping_contains_cycle")
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
		isMapped = true
	}
	return currentModel, isMapped, nil
}
