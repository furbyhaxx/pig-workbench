package config

import (
	"fmt"
	"slices"
)

// ReadPolicy is the typed per-operation read ceiling.
type ReadPolicy struct {
	MaxBytes    int `json:"maxBytes"`
	MaxLines    int `json:"maxLines"`
	Concurrency int `json:"concurrency"`
}

// Entry describes one effective value and its source.
type Entry struct {
	Key         string `json:"key"`
	Value       any    `json:"value"`
	Scope       Scope  `json:"scope"`
	File        string `json:"file,omitempty"`
	Application string `json:"application"`
}

// Snapshot is immutable after publication and may be captured by concurrent work.
type Snapshot struct {
	revision  uint64
	values    map[string]Entry
	documents map[Scope]*document
	warnings  []string
	read      ReadPolicy
}

// Revision identifies the published configuration within this session generation.
func (s *Snapshot) Revision() uint64 { return s.revision }

// Read returns a typed value copy for one operation.
func (s *Snapshot) Read() ReadPolicy { return s.read }

// Get returns an independent copy of a known setting's effective entry.
func (s *Snapshot) Get(key string) (Entry, bool) {
	entry, ok := s.values[key]
	entry.Value = cloneValue(entry.Value)
	return entry, ok
}

// Entries returns independent entries in catalog order.
func (s *Snapshot) Entries() []Entry {
	result := make([]Entry, 0, len(s.values))
	for _, definition := range Catalog() {
		entry, ok := s.Get(definition.Key)
		if ok {
			result = append(result, entry)
		}
	}
	return result
}

// Warnings returns source diagnostics without values.
func (s *Snapshot) Warnings() []string {
	result := slices.Clone(s.warnings)
	if result == nil {
		return []string{}
	}
	return result
}

func compose(documents map[Scope]*document, revision uint64) (*Snapshot, error) {
	snapshot := &Snapshot{revision: revision, values: make(map[string]Entry), documents: documents}
	catalog := Catalog()
	for _, definition := range catalog {
		snapshot.values[definition.Key] = Entry{Key: definition.Key, Value: cloneValue(definition.Default), Scope: Defaults, Application: definition.Application}
	}
	for _, scope := range []Scope{User, Project, Runtime} {
		doc := documents[scope]
		if doc == nil {
			continue
		}
		if scope == Project {
			if _, ok := doc.tree["trust"]; ok {
				return nil, &Error{Path: doc.path, Key: "trust", Reason: "trust rules are user/runtime-only; remove them from project settings"}
			}
		}
		for _, definition := range catalog {
			value, ok := doc.values[definition.Key]
			if !ok {
				continue
			}
			if scope == Project {
				allowed := false
				switch definition.Project {
				case AllowProject:
					allowed = true
				case ReduceProject:
					allowed = value.(int64) <= snapshot.values[definition.Key].Value.(int64)
				case RequireTrueProject:
					allowed = value.(bool)
				}
				if !allowed {
					return nil, &Error{Path: doc.path, Key: definition.Key, Reason: "project settings cannot widen this policy; move the override to user/runtime scope"}
				}
			}
			snapshot.values[definition.Key] = Entry{Key: definition.Key, Value: cloneValue(value), Scope: scope, File: doc.path, Application: definition.Application}
		}
		snapshot.warnings = append(snapshot.warnings, doc.warnings...)
	}
	integer := func(key string) int { return int(snapshot.values[key].Value.(int64)) }
	if integer("read.query.default-rows") > integer("read.query.max-rows") {
		return nil, &Error{Key: "read.query.default-rows", Reason: "default row count must not exceed read.query.max-rows"}
	}
	snapshot.read = ReadPolicy{MaxBytes: integer("read.max-bytes"), MaxLines: integer("read.max-lines"), Concurrency: integer("read.concurrency")}
	return snapshot, nil
}

func cloneValue(value any) any {
	if list, ok := value.([]string); ok {
		return append([]string{}, list...)
	}
	return value
}

func definitions() map[string]Definition {
	result := make(map[string]Definition)
	for _, definition := range Catalog() {
		if _, exists := result[definition.Key]; exists {
			panic("duplicate setting definition: " + definition.Key)
		}
		if _, err := definition.validate(definition.Default); err != nil {
			panic(fmt.Sprintf("invalid default for %s: %v", definition.Key, err))
		}
		result[definition.Key] = definition
	}
	return result
}
