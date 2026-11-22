package main

func cloneStrings(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	return append([]string(nil), paths...)
}

// clearExternalFiles drops item-specific slaves before the next Load. The
// current backend is replaced immediately by each list-step call site.
func clearExternalFiles(c *cell) {
	if c == nil {
		return
	}
	c.externalAudio = nil
	c.externalSubtitles = nil
	if c.eng != nil {
		c.eng.SetExternalFilesForLoad(nil, nil)
	}
}

func rememberExternalFile(paths []string, path string) ([]string, bool) {
	if path == "" {
		return paths, false
	}
	for _, existing := range paths {
		if existing == path {
			return paths, false
		}
	}
	return append(paths, path), true
}
