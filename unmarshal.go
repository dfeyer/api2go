package api2go

import (
	"encoding/json"
	"errors"
	"reflect"
)

type unmarshalContext map[string]interface{}

// Unmarshal reads a JSONAPI map to a model struct
func Unmarshal(ctx unmarshalContext, values interface{}) error {
	// Check that target is a *[]Model
	ptrVal := reflect.ValueOf(values)
	if ptrVal.Kind() != reflect.Ptr || ptrVal.IsNil() {
		panic("You must pass a pointer to a []struct to Unmarshal()")
	}
	sliceType := reflect.TypeOf(values).Elem()
	sliceVal := ptrVal.Elem()
	if sliceType.Kind() != reflect.Slice {
		panic("You must pass a pointer to a []struct to Unmarshal()")
	}
	structType := sliceType.Elem()
	if structType.Kind() != reflect.Struct {
		panic("You must pass a pointer to a []struct to Unmarshal()")
	}

	// Copy the value, then write into the new variable.
	// Later Set() the actual value of the pointee.
	val := sliceVal
	err := unmarshalInto(ctx, structType, &val)
	if err != nil {
		return err
	}
	sliceVal.Set(val)
	return nil
}

func unmarshalInto(ctx unmarshalContext, structType reflect.Type, sliceVal *reflect.Value) error {
	// Read models slice
	rootName := pluralize(jsonify(structType.Name()))
	var modelsInterface interface{}
	if modelsInterface = ctx[rootName]; modelsInterface == nil {
		return errors.New("expected root document to include a '" + rootName + "' key but it didn't.")
	}
	models, ok := modelsInterface.([]interface{})
	if !ok {
		return errors.New("expected slice under key '" + rootName + "'")
	}

	// Read all the models
	for _, m := range models {
		attributes, ok := m.(map[string]interface{})
		if !ok {
			return errors.New("expected an array of objects under key '" + rootName + "'")
		}

		var val reflect.Value
		isNew := true
		id := ""

		if v := attributes["id"]; v != nil {
			id, ok = v.(string)
			if !ok {
				return errors.New("id must be a string")
			}

			// If we have an ID, check if there's already an object with that ID in the slice
			// TODO This is O(n^2), make it O(n)
			for i := 0; i < sliceVal.Len(); i++ {
				obj := sliceVal.Index(i)
				otherID, err := idFromObject(obj)
				if err != nil {
					return err
				}
				if otherID == id {
					val = obj
					isNew = false
					break
				}
			}
		}
		// If the struct wasn't already there for updating, make a new one
		if !val.IsValid() {
			val = reflect.New(structType).Elem()
		}

		for k, v := range attributes {
			switch k {
			case "links":
				linksMap, ok := v.(map[string]interface{})
				if !ok {
					return errors.New("expected links to be an object")
				}
				if err := unmarshalLinks(val, linksMap); err != nil {
					return err
				}

			case "id":
				// Allow conversion of string id to int
				id, ok = v.(string)
				if !ok {
					return errors.New("expected id to be of type string")
				}
				if err := setObjectID(val, id); err != nil {
					return err
				}

			default:
				fieldName := dejsonify(k)
				field := val.FieldByName(fieldName)
				if !field.IsValid() {
					return errors.New("expected struct " + structType.Name() + " to have field " + fieldName)
				}
				field.Set(reflect.ValueOf(v))
			}
		}

		if isNew {
			*sliceVal = reflect.Append(*sliceVal, val)
		}
	}

	return nil
}

func unmarshalLinks(val reflect.Value, linksMap map[string]interface{}) error {
	for linkName, linkObj := range linksMap {
		switch links := linkObj.(type) {
		case []interface{}:
			// Has-many
			// Check for field named 'FoobarsIDs' for key 'foobars'
			structFieldName := dejsonify(linkName) + "IDs"
			sliceField := val.FieldByName(structFieldName)
			if !sliceField.IsValid() || sliceField.Kind() != reflect.Slice {
				return errors.New("expected struct to have a " + structFieldName + " slice")
			}

			sliceField.Set(reflect.MakeSlice(sliceField.Type(), len(links), len(links)))
			for i, idInterface := range links {
				if err := setIDValue(sliceField.Index(i), idInterface); err != nil {
					return err
				}
			}

		case string:
			// Belongs-to or has-one
			// Check for field named 'FoobarID' for key 'foobar'
			structFieldName := dejsonify(linkName) + "ID"
			field := val.FieldByName(structFieldName)
			if err := setIDValue(field, links); err != nil {
				return err
			}

		default:
			return errors.New("expected string or array in links object")
		}
	}
	return nil
}

// UnmarshalFromJSON reads a JSONAPI compatible JSON document to a model struct
func UnmarshalFromJSON(data []byte, values interface{}) error {
	var ctx unmarshalContext
	err := json.Unmarshal(data, &ctx)
	if err != nil {
		return err
	}
	return Unmarshal(ctx, values)
}
