// loader/scanner.go
package loader

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/goef-team/goef/metadata"
)

// scanEntity provides shared scanning logic for both Row and Rows
func (l *Loader) scanEntity(scanner interface{}, meta *metadata.EntityMetadata) (interface{}, error) {
	entity := reflect.New(meta.Type).Interface()
	entityValue := reflect.ValueOf(entity).Elem()

	// Only create scan destinations for non-relationship fields
	var scanDests []interface{}
	var scanFields []metadata.FieldMetadata

	for _, field := range meta.Fields {
		if !field.IsRelationship { // Skip relationship fields
			scanFields = append(scanFields, field)
		}
	}

	scanDests = make([]interface{}, len(scanFields))
	for i, field := range scanFields {
		fieldValue := entityValue.FieldByName(field.Name)
		if !fieldValue.IsValid() {
			return nil, fmt.Errorf("field %s not found in entity", field.Name)
		}
		scanDests[i] = fieldValue.Addr().Interface()
	}

	var err error
	switch s := scanner.(type) {
	case *sql.Row:
		err = s.Scan(scanDests...)
	case *sql.Rows:
		err = s.Scan(scanDests...)
	default:
		return nil, fmt.Errorf("unsupported scanner type: %T", scanner)
	}

	if err != nil {
		return nil, err
	}

	return entity, nil
}

// scanSingleEntity scans a single entity from a sql.Row
func (l *Loader) scanSingleEntity(row *sql.Row, meta *metadata.EntityMetadata) (interface{}, error) {
	return l.scanEntity(row, meta)
}

// scanSingleEntityFromRows scans a single entity from sql.Rows
func (l *Loader) scanSingleEntityFromRows(rows *sql.Rows, meta *metadata.EntityMetadata) (interface{}, error) {
	return l.scanEntity(rows, meta)
}
