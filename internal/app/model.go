package app

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"unicode/utf8"
)

type object = map[string]any

type snapshot struct {
	Firmware     object   `json:"firmware"`
	Options      object   `json:"options"`
	Alternatives object   `json:"alternatives"`
	Profiles     []object `json:"profiles"`
}

type layoutButton struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Size      float64 `json:"size"`
	Shape     string  `json:"shape"`
	Pin       *int    `json:"pin"`
	GroupID   string  `json:"groupId,omitempty"`
	Direction string  `json:"direction,omitempty"`
}

type layout struct {
	Version   int            `json:"version"`
	Name      string         `json:"name"`
	DeviceKey *string        `json:"deviceKey"`
	LabelSet  string         `json:"labelSet"`
	Buttons   []layoutButton `json:"buttons"`
}

var pinPattern = regexp.MustCompile(`^pin\d{2}$`)
var idPattern = regexp.MustCompile(`^[\w-]{1,80}$`)

func integer(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < -9007199254740991 || v > 9007199254740991 {
			return 0, false
		}
		return int64(v), true
	}
	return 0, false
}

func pinKeys(profile object) []string {
	keys := []string{}
	for key := range profile {
		if pinPattern.MatchString(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func same(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

func cloneObject(value object) object {
	copy := make(object, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}

func protectedPin(value any) bool {
	mapping, ok := value.(object)
	if !ok {
		return true
	}
	action, valid := integer(mapping["action"])
	return !valid || action == 0 || action == -5
}

func validateSnapshot(s *snapshot) error {
	if s == nil || len(s.Profiles) < 1 || len(s.Profiles) > 6 {
		return messageError("error.profileFormat")
	}
	keys := pinKeys(s.Profiles[0])
	if len(keys) != 30 && len(keys) != 48 {
		return messageError("error.pinFormat")
	}
	for i, key := range keys {
		if key != fmt.Sprintf("pin%02d", i) {
			return messageError("error.pinTable")
		}
	}
	for _, p := range s.Profiles {
		if !same(pinKeys(p), keys) {
			return messageError("error.pinCount")
		}
		if _, ok := p["profileLabel"].(string); !ok {
			return messageError("error.profileName")
		}
		if _, ok := p["enabled"].(bool); !ok {
			n, valid := integer(p["enabled"])
			if !valid || n < 0 || n > 1 {
				return messageError("error.profileState")
			}
		}
		for _, key := range keys {
			m, ok := p[key].(object)
			if !ok {
				return messageError("error.mappingFormat")
			}
			for _, field := range []string{"action", "customButtonMask", "customDpadMask"} {
				if _, ok := integer(m[field]); !ok {
					return messageError("error.mappingIncomplete")
				}
			}
		}
	}
	return nil
}

func validateMapping(m object) error {
	a, aOK := integer(m["action"])
	b, bOK := integer(m["customButtonMask"])
	d, dOK := integer(m["customDpadMask"])
	if !aOK || !bOK || !dOK {
		return messageError("error.buttonMapping")
	}
	if a == 40 {
		if b < 0 || b > 16383 || d < 0 || d > 15 || b|d == 0 {
			return messageError("error.combo")
		}
		return nil
	}
	if (a != -10 && (a < 1 || a > 19)) || b != 0 || d != 0 {
		return messageError("error.advanced")
	}
	return nil
}

func applyChanges(s *snapshot, index int, changes map[string]object) (*snapshot, error) {
	if err := validateSnapshot(s); err != nil {
		return nil, err
	}
	if index < 0 || index >= len(s.Profiles) || len(changes) == 0 {
		return nil, messageError("error.noChanges")
	}
	next := *s
	next.Profiles = append([]object(nil), s.Profiles...)
	next.Profiles[index] = cloneObject(s.Profiles[index])
	for key, mapping := range changes {
		if !pinPattern.MatchString(key) || s.Profiles[index][key] == nil {
			return nil, messageError("error.pinMissing")
		}
		if protectedPin(s.Profiles[0][key]) || protectedPin(s.Profiles[index][key]) {
			return nil, messageError("error.protected")
		}
		if err := validateMapping(mapping); err != nil {
			return nil, err
		}
		value := cloneObject(s.Profiles[index][key].(object))
		for _, field := range []string{"action", "customButtonMask", "customDpadMask"} {
			value[field] = mapping[field]
		}
		next.Profiles[index][key] = value
	}
	return &next, nil
}

func normalizeLayout(l *layout) error {
	if (l.Version != 1 && l.Version != 2) || utf8.RuneCountInString(l.Name) > 80 || l.Buttons == nil || len(l.Buttons) > 80 {
		return messageError("error.layout")
	}
	if l.DeviceKey != nil && len(*l.DeviceKey) > 1000 {
		return messageError("error.binding")
	}
	ids := map[string]bool{}
	pins := map[int]bool{}
	groups := map[string][]layoutButton{}
	if l.LabelSet == "" {
		l.LabelSet = "xbox"
	}
	switch l.LabelSet {
	case "gp2040", "xbox", "playstation", "switch", "arcade":
	default:
		return messageError("error.labelSet")
	}
	for i := range l.Buttons {
		b := &l.Buttons[i]
		if b.Shape == "" {
			b.Shape = "circle"
		}
		if b.Shape != "dot" && b.Shape != "circle" && b.Shape != "capsule" && b.Shape != "joystick" {
			return messageError("error.shape")
		}
		if b.Shape == "joystick" {
			if !idPattern.MatchString(b.GroupID) {
				return messageError("error.stickGroup")
			}
			switch b.Direction {
			case "up", "down", "left", "right":
			default:
				return messageError("error.stickDirection")
			}
			for _, other := range groups[b.GroupID] {
				if other.Direction == b.Direction || other.X != b.X || other.Y != b.Y || other.Size != b.Size {
					return messageError("error.stickConsistency")
				}
			}
			groups[b.GroupID] = append(groups[b.GroupID], *b)
		} else if b.GroupID != "" || b.Direction != "" {
			return messageError("error.normalDirection")
		}
		minSize, maxSize := 2.0, 20.0
		if b.Shape == "joystick" {
			minSize, maxSize = 12, 40
		}
		if !idPattern.MatchString(b.ID) || ids[b.ID] || utf8.RuneCountInString(b.Name) > 40 || b.X < 0 || b.X > 100 || b.Y < 0 || b.Y > 100 || b.Size < minSize || b.Size > maxSize {
			return messageError("error.position")
		}
		if math.IsNaN(b.X+b.Y+b.Size) || math.IsInf(b.X+b.Y+b.Size, 0) {
			return messageError("error.position")
		}
		ids[b.ID] = true
		if b.Pin != nil {
			if *b.Pin < 0 || *b.Pin > 47 || pins[*b.Pin] {
				return messageError("error.duplicatePin")
			}
			pins[*b.Pin] = true
		}
	}
	for _, group := range groups {
		if len(group) != 4 {
			return messageError("error.stickIncomplete")
		}
	}
	l.Version = 2
	return nil
}
