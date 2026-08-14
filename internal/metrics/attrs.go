package metrics

import "go.opentelemetry.io/otel/attribute"

var (
	attrOperationCreate = attribute.String("operation", "create")
	attrOperationUpdate = attribute.String("operation", "update")
	attrOperationDelete = attribute.String("operation", "delete")
)
