package app

import "testing"

func joystickLayout() object {
	buttons := []any{}
	for i, direction := range []string{"up", "down", "left", "right"} {
		buttons = append(buttons, object{"id": direction, "name": direction, "x": 25, "y": 50, "size": 26, "shape": "joystick", "groupId": "stick-1", "direction": direction, "pin": i + 4})
	}
	return object{"version": 2, "name": "摇杆布局", "deviceKey": "test-device", "labelSet": "playstation", "buttons": buttons}
}

func TestJoystickAndLabelPreferenceSurviveRestart(t *testing.T) {
	f := newFixture(t, 30)
	l := joystickLayout()
	for _, label := range []string{"gp2040", "xbox", "playstation", "switch", "arcade"} {
		l["labelSet"] = label
		status, out := f.call("layout", l, "PUT", "")
		if status != 200 {
			t.Fatalf("save %s: %d %v", label, status, out)
		}
		f.a = newApp(f.a.dataDir, nil)
		status, out = f.call("layout", nil, "GET", "")
		if status != 200 || !same(out, l) {
			t.Fatalf("restore %s: %v", label, out)
		}
	}
	if len(f.writes) != 0 {
		t.Fatal("display preference wrote device")
	}
}

func TestInvalidJoystickDoesNotReplaceSavedLayout(t *testing.T) {
	f := newFixture(t, 30)
	valid := joystickLayout()
	f.call("layout", valid, "PUT", "")
	cases := map[string]func(object){
		"missing direction":   func(l object) { l["buttons"] = l["buttons"].([]any)[:3] },
		"duplicate direction": func(l object) { l["buttons"].([]any)[1].(object)["direction"] = "up" },
		"invalid direction":   func(l object) { l["buttons"].([]any)[1].(object)["direction"] = "diagonal" },
		"split position":      func(l object) { l["buttons"].([]any)[1].(object)["x"] = 75 },
		"split size":          func(l object) { l["buttons"].([]any)[1].(object)["size"] = 30 },
		"duplicate GPIO":      func(l object) { l["buttons"].([]any)[1].(object)["pin"] = 4 },
		"mixed shape":         func(l object) { l["buttons"].([]any)[1].(object)["shape"] = "circle" },
		"invalid scheme":      func(l object) { l["labelSet"] = "unknown" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			bad := deepCopy(valid)
			mutate(bad)
			if status, _ := f.call("layout", bad, "PUT", ""); status != 400 {
				t.Fatal("accepted malformed layout")
			}
			_, got := f.call("layout", nil, "GET", "")
			if !same(got, valid) {
				t.Fatal("invalid layout changed saved file")
			}
		})
	}
}

func TestA2SingleAndCombinationSave(t *testing.T) {
	for _, output := range []object{mapping(16), {"action": 40, "customButtonMask": 8193, "customDpadMask": 1}} {
		f := newFixture(t, 30)
		before := deepCopy(f.base)
		status, result := f.save(f.connect(t), 0, object{"pin05": output})
		if status != 200 {
			t.Fatalf("save A2: %d %v", status, result)
		}
		expected := deepCopy(before)
		for field, value := range output {
			expected["pin05"].(object)[field] = value
		}
		if !same(expected, f.base) || len(f.writes) != 1 {
			t.Fatal("A2 changed unrelated mappings or fields")
		}
	}
}
