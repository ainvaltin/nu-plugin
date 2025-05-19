package nu

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

/*
ToValue returns canonical Nu Value of the v.

Supported input types are:
  - int and uint types
  - float32 and float64
  - time.Duration and time.Time
  - string
  - []byte
  - Nu types defined by this package: [IntRange], [Record], [Filesize], [Glob], [Block], [Closure], [CellPath], []Value
  - nil

Slices and arrays (other than byte slices) are converted to List.

Maps and structs are converted to Record.

In case of unsupported type the Value returned will contain error.
*/
func ToValue(v any) Value {
	switch t := v.(type) {
	case nil:
		return Value{Value: nil}
	case int:
		return Value{Value: int64(t)}
	case int8:
		return Value{Value: int64(t)}
	case int16:
		return Value{Value: int64(t)}
	case int32:
		return Value{Value: int64(t)}
	case int64:
		return Value{Value: t}
	case uint:
		if t > math.MaxInt64 {
			return Value{Value: fmt.Errorf("uint %d is too large for int64", t)}
		}
		return Value{Value: int64(t)}
	case uint8:
		return Value{Value: int64(t)}
	case uint16:
		return Value{Value: int64(t)}
	case uint32:
		return Value{Value: int64(t)}
	case uint64:
		if t > math.MaxInt64 {
			return Value{Value: fmt.Errorf("uint %d is too large for int64", t)}
		}
		return Value{Value: int64(t)}
	case float32:
		return Value{Value: float64(t)}
	case float64:
		return Value{Value: t}
	case string, []byte:
		return Value{Value: v}
	case time.Duration, time.Time:
		return Value{Value: v}
	case map[string]Value:
		return Value{Value: Record(t)}
	case IntRange, Record, Filesize, Glob, Block, Closure:
		return Value{Value: v}
	case []Value:
		return Value{Value: v}
	case error:
		return Value{Value: v}
	case CustomValue:
		return Value{Value: v}
	case CellPath:
		return Value{Value: v}
	case Value:
		return t
	default:
		return rv2nv(reflect.ValueOf(v))
	}
}

func rv2nv(v reflect.Value) Value {
	if v.IsValid() && v.Type().Implements(reflect.TypeFor[CustomValue]()) {
		return Value{Value: v.Interface()}
	}

	switch v.Kind() {
	case reflect.Bool:
		return Value{Value: v.Bool()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Value{Value: v.Int()}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return Value{Value: int64(v.Uint())}
	case reflect.Uint, reflect.Uint64:
		i := v.Uint()
		if i > math.MaxInt64 {
			return Value{Value: fmt.Errorf("uint %d is too large for int64", i)}
		}
		return Value{Value: int64(i)}
	case reflect.Float32, reflect.Float64:
		return Value{Value: v.Float()}
	case reflect.String:
		return Value{Value: v.String()}
	case reflect.Interface:
		return rv2nv(v.Elem())
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[CellPath]() {
			return Value{Value: v.Interface()}
		}

		r := Record{}
		for i := range v.Type().NumField() {
			f := v.Type().Field(i)
			r[f.Name] = rv2nv(v.FieldByIndex(f.Index))
		}
		return Value{Value: r}
	case reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if v.CanAddr() {
				return Value{Value: v.Bytes()}
			}
			r := make([]byte, v.Len())
			for i := range v.Len() {
				r[i] = byte(v.Index(i).Uint())
			}
			return Value{Value: r}
		}

		r := make([]Value, v.Len())
		for i := range v.Len() {
			r[i] = rv2nv(v.Index(i))
		}
		return Value{Value: r}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return Value{Value: v.Bytes()}
		}

		r := make([]Value, v.Len())
		for i := range v.Len() {
			r[i] = rv2nv(v.Index(i))
		}
		return Value{Value: r}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return Value{Value: fmt.Errorf("map key type must be string, got %s", v.Type())}
		}

		r := Record{}
		for iter := v.MapRange(); iter.Next(); {
			r[iter.Key().String()] = rv2nv(iter.Value())
		}
		return Value{Value: r}
	case reflect.Invalid:
		if !v.IsValid() {
			return Value{Value: nil}
		}
		return Value{Value: fmt.Errorf("unsupported value type %v", v)}
	default:
		return Value{Value: fmt.Errorf("unsupported value type %s", v.Type())}
	}
}
