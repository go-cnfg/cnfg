package cnfg

import "reflect"

// clone returns a deep copy of cfg, one that shares no pointer, map or slice with it, so
// that nothing a parser writes reaches the defaults the caller still holds.
func clone[T any](cfg T) T {
	c, _ := cloneValue(reflect.ValueOf(&cfg).Elem()).Interface().(T)
	return c
}

// cloneValue copies v along with everything reachable through its exported fields.
// Unexported fields come along with their struct as they are, since cnfg never sets them.
func cloneValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(cloneValue(v.Elem()))
		return p
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		return cloneValue(v.Elem())
	case reflect.Struct:
		return cloneStruct(v)
	case reflect.Map:
		return cloneMap(v)
	case reflect.Slice:
		return cloneSlice(v)
	default:
		return v
	}
}

func cloneStruct(v reflect.Value) reflect.Value {
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	for i := range c.NumField() {
		if f := c.Field(i); f.CanSet() {
			f.Set(cloneValue(f))
		}
	}
	return c
}

func cloneMap(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return v
	}
	m := reflect.MakeMapWithSize(v.Type(), v.Len())
	for _, k := range v.MapKeys() {
		m.SetMapIndex(k, cloneValue(v.MapIndex(k)))
	}
	return m
}

func cloneSlice(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return v
	}
	s := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
	for i := range v.Len() {
		s.Index(i).Set(cloneValue(v.Index(i)))
	}
	return s
}
