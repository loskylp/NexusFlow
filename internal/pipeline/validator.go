// Package pipeline provides design-time validation for pipeline definitions.
// See: ADR-008, TASK-026, REQ-007
package pipeline

import (
	"fmt"

	"github.com/nxlabs/nexusflow/internal/models"
)

// ValidateSchemaMappings checks that all schema mappings in the pipeline refer to
// fields that exist in the preceding phase's declared OutputSchema.
//
// Two transitions are validated:
//   - DataSource -> Process: each ProcessConfig.InputMappings[*].SourceField must
//     exist in DataSourceConfig.OutputSchema.
//   - Process -> Sink: each SinkConfig.InputMappings[*].SourceField must exist in
//     ProcessConfig.OutputSchema.
//
// Empty mappings are valid — when no mappings are declared there is nothing to check.
// A non-empty mapping list against an empty OutputSchema is invalid because the
// source fields cannot be satisfied.
//
// Returns the first mapping violation found, or nil when all mappings are valid.
// Error format: "<phase> input mapping: source field '<field>' not found in <preceding phase> output schema"
//
// Preconditions: pipeline contains fully decoded DataSourceConfig, ProcessConfig, and SinkConfig.
// Postconditions: nil return means all declared mappings reference existing source fields.
func ValidateSchemaMappings(p models.Pipeline) error {
	if err := validateProcessInputMappings(p.ProcessConfig.InputMappings, p.DataSourceConfig.OutputSchema); err != nil {
		return err
	}
	return validateSinkInputMappings(p.SinkConfig.InputMappings, p.ProcessConfig.OutputSchema)
}

// validateProcessInputMappings checks that every SourceField in the process input
// mappings exists in the datasource output schema.
//
// Returns an error naming the first missing field, or nil if all fields are present.
func validateProcessInputMappings(mappings []models.SchemaMapping, datasourceOutputSchema []string) error {
	return validateMappings(mappings, datasourceOutputSchema, "process input mapping", "datasource output schema")
}

// validateSinkInputMappings checks that every SourceField in the sink input
// mappings exists in the process output schema.
//
// Returns an error naming the first missing field, or nil if all fields are present.
func validateSinkInputMappings(mappings []models.SchemaMapping, processOutputSchema []string) error {
	return validateMappings(mappings, processOutputSchema, "sink input mapping", "process output schema")
}

// validateMappings checks that every SourceField in mappings exists in the given
// schema slice. It builds a set from schema for O(1) lookup, then iterates mappings
// in order so the first violation is reported deterministically.
//
// mappingContext and schemaContext are used verbatim in the error message to
// identify which pipeline transition failed.
//
// Returns an error naming the first missing SourceField, or nil when all are found.
func validateMappings(
	mappings []models.SchemaMapping,
	schema []string,
	mappingContext string,
	schemaContext string,
) error {
	if len(mappings) == 0 {
		return nil
	}

	available := buildFieldSet(schema)
	for _, m := range mappings {
		if !available[m.SourceField] {
			return fmt.Errorf("%s: source field %q not found in %s", mappingContext, m.SourceField, schemaContext)
		}
	}
	return nil
}

// buildFieldSet converts a schema slice into a map for O(1) membership testing.
// An empty or nil schema produces an empty set.
func buildFieldSet(schema []string) map[string]bool {
	set := make(map[string]bool, len(schema))
	for _, field := range schema {
		set[field] = true
	}
	return set
}
