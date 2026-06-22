package service

// DeepMerge deeply merges src into dst. Map values are merged recursively; leaf
// values from src overwrite the corresponding destination value. Returns dst.
func DeepMerge(dst, src any) any {
	dstCopy, _ := castToMap(copyInterfaceToMapStringInterface(dst))
	srcMap, ok := castToMap(src)
	if !ok {
		return src // at leaves, simply replace
	}
	for k, v := range srcMap {
		if existing, exists := dstCopy[k]; exists {
			dstCopy[k] = DeepMerge(existing, v)
		} else {
			dstCopy[k] = copyInterfaceToMapStringInterface(v)
		}
	}
	return dstCopy
}

// deepObjectMerge deep-merges src into dst in-place. Map values are merged recursively;
// leaf values from src overwrite the corresponding destination value.
func deepObjectMerge(dst, src map[string]any) {
	for k, v := range src {
		if existing, ok := dst[k].(map[string]any); ok {
			if nextSrc, ok := v.(map[string]any); ok && nextSrc != nil {
				deepObjectMerge(existing, nextSrc)
			} else {
				dst[k] = copyInterfaceToMapStringInterface(v)
			}
		} else {
			dst[k] = copyInterfaceToMapStringInterface(v)
		}
	}
}

// castToMap attempts to convert val to map[string]any.
// Returns the map and true if conversion succeeded, nil and false otherwise.
func castToMap(val any) (map[string]any, bool) {
	if m, ok := val.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

// copyInterfaceToMapStringInterface converts val to a deep-copied
// map[string]any. Strings, numbers, booleans and slices pass through.
func copyInterfaceToMapStringInterface(val any) any {
	if sv, ok := val.(map[string]any); ok {
		copied := make(map[string]any, len(sv))
		for k, v := range sv {
			copied[k] = copyInterfaceToMapStringInterface(v)
		}
		return copied
	}
	return val
}

// stripMetadata removes internal-only keys from a variant spec entry before merging
// into LiteLLM deployment params. Keys like "suffix"/"prefix" are used only to derive
// the deployment name and must never reach LiteLLM's config.
func stripMetadata(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		switch k {
		case "suffix", "prefix":
			continue // skip - these are only for naming
		default:
			out[k] = copyInterfaceToMapStringInterface(v)
		}
	}
	return out
}

// mapDiffers returns true if two string-to-string maps differ in length or any key-value pair.
func mapDiffers(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || v != bv {
			return true
		}
	}
	return false
}
