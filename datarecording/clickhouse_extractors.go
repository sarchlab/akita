package datarecording

import (
	"fmt"
	"reflect"
)

// These extractors use minimal reflection ONLY as a fallback
// The fast path uses type assertions in the convert functions

func extractTaskTableEntry(entry any) taskTableEntryDB {
	v := reflect.ValueOf(entry)

	// Validate this is the right structure
	if v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("expected struct for task entry, got %T", entry))
	}

	result := taskTableEntryDB{}

	// Extract fields by name
	if field := v.FieldByName("ID"); field.IsValid() {
		result.ID = field.String()
	}
	if field := v.FieldByName("ParentID"); field.IsValid() {
		result.ParentID = field.String()
	}
	if field := v.FieldByName("Kind"); field.IsValid() {
		result.Kind = field.String()
	}
	if field := v.FieldByName("What"); field.IsValid() {
		result.What = field.String()
	}
	if field := v.FieldByName("Location"); field.IsValid() {
		result.Location = field.String()
	}
	if field := v.FieldByName("StartTime"); field.IsValid() {
		result.StartTime = field.Float()
	}
	if field := v.FieldByName("EndTime"); field.IsValid() {
		result.EndTime = field.Float()
	}

	return result
}

func extractMilestoneTableEntry(entry any) milestoneTableEntryDB {
	v := reflect.ValueOf(entry)

	if v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("expected struct for milestone entry, got %T", entry))
	}

	result := milestoneTableEntryDB{}

	if field := v.FieldByName("ID"); field.IsValid() {
		result.ID = field.String()
	}
	if field := v.FieldByName("TaskID"); field.IsValid() {
		result.TaskID = field.String()
	}
	if field := v.FieldByName("Time"); field.IsValid() {
		result.Time = field.Float()
	}
	if field := v.FieldByName("Kind"); field.IsValid() {
		result.Kind = field.String()
	}
	if field := v.FieldByName("What"); field.IsValid() {
		result.What = field.String()
	}
	if field := v.FieldByName("Location"); field.IsValid() {
		result.Location = field.String()
	}

	return result
}

func extractSegmentTableEntry(entry any) segmentTableEntryDB {
	v := reflect.ValueOf(entry)

	if v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("expected struct for segment entry, got %T", entry))
	}

	result := segmentTableEntryDB{}

	if field := v.FieldByName("StartTime"); field.IsValid() {
		result.StartTime = field.Float()
	}
	if field := v.FieldByName("EndTime"); field.IsValid() {
		result.EndTime = field.Float()
	}

	return result
}

func extractMemoryTransactionEntry(entry any) memoryTransactionEntryDB {
	v := reflect.ValueOf(entry)

	if v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("expected struct for memory transaction entry, got %T", entry))
	}

	result := memoryTransactionEntryDB{}

	if field := v.FieldByName("ID"); field.IsValid() {
		result.ID = field.String()
	}
	if field := v.FieldByName("Location"); field.IsValid() {
		result.Location = field.String()
	}
	if field := v.FieldByName("What"); field.IsValid() {
		result.What = field.String()
	}
	if field := v.FieldByName("StartTime"); field.IsValid() {
		result.StartTime = field.Float()
	}
	if field := v.FieldByName("EndTime"); field.IsValid() {
		result.EndTime = field.Float()
	}
	if field := v.FieldByName("Address"); field.IsValid() {
		result.Address = field.Uint()
	}
	if field := v.FieldByName("ByteSize"); field.IsValid() {
		result.ByteSize = field.Uint()
	}

	return result
}

func extractMemoryStepEntry(entry any) memoryStepEntryDB {
	v := reflect.ValueOf(entry)

	if v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("expected struct for memory step entry, got %T", entry))
	}

	result := memoryStepEntryDB{}

	if field := v.FieldByName("ID"); field.IsValid() {
		result.ID = field.String()
	}
	if field := v.FieldByName("TaskID"); field.IsValid() {
		result.TaskID = field.String()
	}
	if field := v.FieldByName("Time"); field.IsValid() {
		result.Time = field.Float()
	}
	if field := v.FieldByName("What"); field.IsValid() {
		result.What = field.String()
	}

	return result
}
