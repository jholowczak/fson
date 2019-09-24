// Package fson provides a way to interact with arbitrary JSON fields as well as use JSON(b) within Postgresql
// Fson supports the json interfaces MarshalJSON/UnmarshalJSON as well as the Scan interface within the db packages
package fson

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Fson struct is the core structure, no exported members
type Fson struct {
	data map[string]interface{}
}

func (self *Fson) MarshalJSON() ([]byte, error) {
	return json.Marshal(self.data)
}

func (self *Fson) UnmarshalJSON(b []byte) error {
	return self.Loads(b)
}

func (self *Fson) Scan(src interface{}) error {
	return self.Loads(src.([]byte))
}

func (self Fson) Bytes() []byte {
	if b, err := json.Marshal(self.data); err == nil {
		return b
	}
	return []byte{}
}

func (self Fson) String() string {
	return string(self.Bytes())
}

func New(b []byte) *Fson {
	if b == nil {
		return &Fson{make(map[string]interface{})}
	} else {
		f := &Fson{}
		f.Loads(b)
		return f
	}
}

// Loads will take nil or a []byte array and create a Fson object with it.
// Generally nil is used if you want to build your JSON object from scratch with the Set method
func (self *Fson) Loads(b []byte) error {
	self.data = make(map[string]interface{})
	if b == nil {
		return nil
	}
	if err := json.Unmarshal(b, &self.data); err != nil {
		return err
	}
	return nil
}

func (self *Fson) set(path []string, value interface{}, cur map[string]interface{}, appendList bool) {
	// check if we are where the insert should happen
	if len(path) == 1 {
		if appendList {
			//check if something is already there
			if _, ok := cur[path[0]]; ok {
				switch cur[path[0]].(type) {
				case []interface{}:
					// add new value to end of current list
					cur[path[0]] = append(cur[path[0]].([]interface{}), value)
				default:
					// it is some singleton value, create a list of it and append new value to it
					temp := cur[path[0]]
					cur[path[0]] = make([]interface{}, 0, 2)
					cur[path[0]] = append(cur[path[0]].([]interface{}), temp)
					cur[path[0]] = append(cur[path[0]].([]interface{}), value)
				}
			}
		} else {
			cur[path[0]] = value
		}
	} else {
		if _, ok := cur[path[0]]; !ok {
			cur[path[0]] = make(map[string]interface{})
		}
		self.set(path[1:], value, cur[path[0]].(map[string]interface{}), appendList)
	}
}

// Set will set a single key within the JSON structure. The key definition is
// defined by a list of strings. Each item in the lest is the parent Object key
// of the next. For example, if we have the json structure:
// {"foo": {"bar": 10}}
// and we want to add a 100 to the key "baz" in the "foo" object, call set with the following form
// fsonobj := fson.New([]byte({\"foo\": {\"bar\": 10}}))
// fsonobj.Set([]string{"foo", "baz"}, 100)
func (self *Fson) Set(path []string, value interface{}, appendList bool) {
	self.set(path, value, self.data, appendList)
}

func (self *Fson) get(path []string, cur map[string]interface{}) interface{} {
	if len(path) == 1 {
		if _, ok := cur[path[0]]; ok {
			return cur[path[0]]
		} else {
			return nil
		}
	}

	if _, ok := cur[path[0]].(map[string]interface{}); ok {
		return self.get(path[1:], cur[path[0]].(map[string]interface{}))
	} else {
		return nil
	}
}

// Get works like set in that the path to the key is specified as a list of
// string values which represent the orderd nested object keys
func (self *Fson) Get(path []string) (interface{}, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("No path specified")
	}

	if v := self.get(path, self.data); v == nil {
		return nil, fmt.Errorf("path not found: %v", path)
	} else {
		return v, nil
	}
}

// Simple helper method that wraps Get but provides a simpler syntax for making Get calls. Path keys are sperated by forward slashes ("rootkey/subkey/subkey")
func (self *Fson) GetP(path string) (interface{}, error) {
	newPath := strings.Split(path, "/")
	return self.Get(newPath)
}

// Simple helper method that wraps Get but provides a simpler syntax for making Get calls. Path keys are sperated by forward slashes ("rootkey.subkey.subkey")
func (self *Fson) GetD(path string) (interface{}, error) {
	newPath := strings.Split(path, ".")
	return self.Get(newPath)
}

func (self *Fson) GetArray(path []string) ([]interface{}, error) {
	data, err := self.Get(path)
	if err != nil {
		return nil, err
	}

	switch data.(type) {
	case []interface{}:
		return data.([]interface{}), nil
	}

	return nil, fmt.Errorf("Data fetched is not a list")
}

type fmapFn func(interface{}) interface{}

func (self *Fson) fmap(f fmapFn, value interface{}) interface{} {
	switch value.(type) {
	case []interface{}:
		lst := make([]interface{}, 0, 0)
		for _, item := range value.([]interface{}) {
			item = self.fmap(f, item)
			lst = append(lst, item)
		}
		return lst
	case map[string]interface{}:
		mp := make(map[string]interface{})
		for k, val := range value.(map[string]interface{}) {
			mp[k] = self.fmap(f, val)
		}
		return mp
	default:
		return f(value)
	}
}

// Fmap applys a function to every value in the JSON structure, mutating them in place
func (self *Fson) Fmap(f fmapFn) {
	mp := make(map[string]interface{})
	for k, v := range self.data {
		mp[k] = self.fmap(f, v)
	}
	self.data = mp
}

func (self *Fson) del(path []string, cur interface{}) interface{} {
	switch cur.(type) {
	case map[string]interface{}:
		mp := make(map[string]interface{})
		if len(path) == 1 {
			for key, val := range cur.(map[string]interface{}) {
				if key != path[0] {
					mp[key] = val
				}
			}
		} else {
			for key, val := range cur.(map[string]interface{}) {
				mp[key] = self.del(path[1:], val)
			}
		}
		return mp
	default:
		return cur
	}
}

// Del will delete a key from the JSON object
func (self *Fson) Del(path []string) {
	mp := make(map[string]interface{})
	for k, v := range self.data {
		if len(path) == 1 {
			if k != path[0] {
				mp[k] = v
			}
		} else {
			if k == path[0] {
				mp[k] = self.del(path[1:], v)
			} else {
				mp[k] = v
			}
		}
	}
	self.data = mp

}

// DelP is a helper method for Del using forward slash as the path separator
func (self *Fson) DelP(path string) {
	self.Del(strings.Split(path, "/"))
}

// DelD is a helper method for Del using dot as the path separator
func (self *Fson) DelD(path string) {
	self.Del(strings.Split(path, "."))
}
