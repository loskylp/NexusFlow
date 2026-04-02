// Package pipeline — unit tests for ValidateSchemaMappings.
// See: ADR-008, TASK-026, REQ-007
package pipeline

import (
	"strings"
	"testing"

	"github.com/nxlabs/nexusflow/internal/models"
)

// --- helpers ---

// pipelineWith builds a Pipeline value from the supplied configs for test use.
func pipelineWith(ds models.DataSourceConfig, proc models.ProcessConfig, sink models.SinkConfig) models.Pipeline {
	return models.Pipeline{
		DataSourceConfig: ds,
		ProcessConfig:    proc,
		SinkConfig:       sink,
	}
}

// --- ValidateSchemaMappings tests ---

// TestValidate_EmptyMappingsPass verifies that a pipeline with no mappings is valid.
// AC-2: valid schema mappings pass.
func TestValidate_EmptyMappingsPass(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"field1", "field2"}},
		models.ProcessConfig{InputMappings: nil, OutputSchema: []string{"out1"}},
		models.SinkConfig{InputMappings: nil},
	)

	if err := ValidateSchemaMappings(p); err != nil {
		t.Errorf("expected no error for empty mappings, got: %v", err)
	}
}

// TestValidate_ValidProcessInputMappingPasses verifies that a mapping whose SourceField
// exists in the DataSource OutputSchema is accepted.
// AC-2, AC-3: DataSource->Process transition validated and passes.
func TestValidate_ValidProcessInputMappingPasses(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"userId", "amount"}},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "userId", TargetField: "uid"},
				{SourceField: "amount", TargetField: "value"},
			},
			OutputSchema: []string{"uid", "value"},
		},
		models.SinkConfig{InputMappings: nil},
	)

	if err := ValidateSchemaMappings(p); err != nil {
		t.Errorf("expected no error for valid process mappings, got: %v", err)
	}
}

// TestValidate_ValidSinkInputMappingPasses verifies that a mapping whose SourceField
// exists in the Process OutputSchema is accepted.
// AC-2, AC-3: Process->Sink transition validated and passes.
func TestValidate_ValidSinkInputMappingPasses(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"raw"}},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "raw", TargetField: "processed"},
			},
			OutputSchema: []string{"processed"},
		},
		models.SinkConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "processed", TargetField: "dest"},
			},
		},
	)

	if err := ValidateSchemaMappings(p); err != nil {
		t.Errorf("expected no error for valid sink mappings, got: %v", err)
	}
}

// TestValidate_MissingProcessSourceFieldReturnsError verifies that a process input
// mapping referencing a field absent from DataSource OutputSchema returns an error
// that names the missing field.
// AC-1, AC-3, AC-4: invalid DS->Process mapping returns 400 identifying the field.
func TestValidate_MissingProcessSourceFieldReturnsError(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"userId"}},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "userId", TargetField: "uid"},
				{SourceField: "missing_field", TargetField: "x"},
			},
			OutputSchema: []string{"uid"},
		},
		models.SinkConfig{},
	)

	err := ValidateSchemaMappings(p)
	if err == nil {
		t.Fatal("expected error for missing process source field, got nil")
	}
	if !strings.Contains(err.Error(), "missing_field") {
		t.Errorf("error message should name the missing field; got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "process input mapping") {
		t.Errorf("error message should identify the process input mapping context; got: %s", err.Error())
	}
}

// TestValidate_MissingSinkSourceFieldReturnsError verifies that a sink input mapping
// referencing a field absent from Process OutputSchema returns an error that names
// the missing field.
// AC-1, AC-3, AC-4: invalid Process->Sink mapping returns 400 identifying the field.
func TestValidate_MissingSinkSourceFieldReturnsError(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"raw"}},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "raw", TargetField: "processed"},
			},
			OutputSchema: []string{"processed"},
		},
		models.SinkConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "processed", TargetField: "dest"},
				{SourceField: "ghost_field", TargetField: "x"},
			},
		},
	)

	err := ValidateSchemaMappings(p)
	if err == nil {
		t.Fatal("expected error for missing sink source field, got nil")
	}
	if !strings.Contains(err.Error(), "ghost_field") {
		t.Errorf("error message should name the missing field; got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "sink input mapping") {
		t.Errorf("error message should identify the sink input mapping context; got: %s", err.Error())
	}
}

// TestValidate_EmptyOutputSchemaWithProcessMappingFails verifies that non-empty
// ProcessConfig.InputMappings with an empty DataSource OutputSchema is rejected.
// AC-1: invalid mapping (no source schema to map from) returns error.
func TestValidate_EmptyOutputSchemaWithProcessMappingFails(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: nil},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "field1", TargetField: "f"},
			},
			OutputSchema: []string{},
		},
		models.SinkConfig{},
	)

	err := ValidateSchemaMappings(p)
	if err == nil {
		t.Fatal("expected error when datasource OutputSchema is empty but process mappings exist, got nil")
	}
	if !strings.Contains(err.Error(), "field1") {
		t.Errorf("error message should name the missing field; got: %s", err.Error())
	}
}

// TestValidate_EmptyOutputSchemaWithSinkMappingFails verifies that non-empty
// SinkConfig.InputMappings with an empty Process OutputSchema is rejected.
// AC-1: invalid mapping (no source schema to map from) returns error.
func TestValidate_EmptyOutputSchemaWithSinkMappingFails(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"x"}},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "x", TargetField: "y"},
			},
			OutputSchema: nil,
		},
		models.SinkConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "y", TargetField: "z"},
			},
		},
	)

	err := ValidateSchemaMappings(p)
	if err == nil {
		t.Fatal("expected error when process OutputSchema is empty but sink mappings exist, got nil")
	}
	if !strings.Contains(err.Error(), "y") {
		t.Errorf("error message should name the missing field; got: %s", err.Error())
	}
}

// TestValidate_BothTransitionsValidPass verifies that a fully-specified pipeline
// with valid mappings on both transitions passes without error.
// AC-2, AC-3: both DS->Process and Process->Sink are validated and pass.
func TestValidate_BothTransitionsValidPass(t *testing.T) {
	p := pipelineWith(
		models.DataSourceConfig{OutputSchema: []string{"a", "b"}},
		models.ProcessConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "a", TargetField: "alpha"},
				{SourceField: "b", TargetField: "beta"},
			},
			OutputSchema: []string{"alpha", "beta"},
		},
		models.SinkConfig{
			InputMappings: []models.SchemaMapping{
				{SourceField: "alpha", TargetField: "x"},
				{SourceField: "beta", TargetField: "y"},
			},
		},
	)

	if err := ValidateSchemaMappings(p); err != nil {
		t.Errorf("expected no error for fully valid pipeline, got: %v", err)
	}
}
