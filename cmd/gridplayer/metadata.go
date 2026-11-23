package main

import "encoding/json"

func cloneRawMessages(in map[string]json.RawMessage) map[string]json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, raw := range in {
		out[key] = append(json.RawMessage(nil), raw...)
	}
	return out
}
