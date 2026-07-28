package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository/db"
)

func replaceTags(ctx context.Context, q db.Querier, dialect db.Dialect, kind model.ParentKind, resourceID uuid.UUID, tags []model.Tag) error {
	deleteQuery := fmt.Sprintf(
		"DELETE FROM tags WHERE resource_kind = %s AND resource_uuid = %s",
		placeholder(dialect, 1),
		placeholder(dialect, 2),
	)
	if _, err := q.ExecContext(ctx, deleteQuery, string(kind), uuidString(dialect, resourceID)); err != nil {
		return fmt.Errorf("delete existing tags: %w", err)
	}

	for _, tag := range tags {
		insertQuery := fmt.Sprintf(
			"INSERT INTO tags (resource_kind, resource_uuid, key, value) VALUES (%s, %s, %s, %s)",
			placeholder(dialect, 1),
			placeholder(dialect, 2),
			placeholder(dialect, 3),
			placeholder(dialect, 4),
		)
		if _, err := q.ExecContext(ctx, insertQuery, string(kind), uuidString(dialect, resourceID), tag.Key, tag.Value); err != nil {
			return fmt.Errorf("insert tag %q: %w", tag.Key, err)
		}
	}

	return nil
}

func loadTagsForResources(
	ctx context.Context,
	q db.Querier,
	dialect db.Dialect,
	kind model.ParentKind,
	resourceIDs []uuid.UUID,
) (map[uuid.UUID][]model.Tag, error) {
	result := make(map[uuid.UUID][]model.Tag, len(resourceIDs))
	if len(resourceIDs) == 0 {
		return result, nil
	}

	args := make([]any, 0, len(resourceIDs)+1)
	args = append(args, string(kind))
	placeholders := make([]string, 0, len(resourceIDs))
	for i, id := range resourceIDs {
		placeholders = append(placeholders, placeholder(dialect, i+2))
		args = append(args, uuidString(dialect, id))
	}

	query := fmt.Sprintf(`
		SELECT resource_uuid, key, value
		FROM tags
		WHERE resource_kind = %s AND resource_uuid IN (%s)
		ORDER BY resource_uuid, key
	`, placeholder(dialect, 1), joinComma(placeholders))

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			rawUUID any
			tag     model.Tag
		)
		if err := rows.Scan(&rawUUID, &tag.Key, &tag.Value); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}

		resourceID, err := scanUUID(dialect, rawUUID)
		if err != nil {
			return nil, err
		}
		result[resourceID] = append(result[resourceID], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	return result, nil
}

func loadTags(ctx context.Context, q db.Querier, dialect db.Dialect, kind model.ParentKind, resourceID uuid.UUID) ([]model.Tag, error) {
	tagMap, err := loadTagsForResources(ctx, q, dialect, kind, []uuid.UUID{resourceID})
	if err != nil {
		return nil, err
	}
	return tagMap[resourceID], nil
}

func deleteTagsForResource(ctx context.Context, q db.Querier, dialect db.Dialect, kind model.ParentKind, resourceID uuid.UUID) error {
	query := fmt.Sprintf(
		"DELETE FROM tags WHERE resource_kind = %s AND resource_uuid = %s",
		placeholder(dialect, 1),
		placeholder(dialect, 2),
	)
	if _, err := q.ExecContext(ctx, query, string(kind), uuidString(dialect, resourceID)); err != nil {
		return fmt.Errorf("delete tags: %w", err)
	}
	return nil
}

func uuidString(dialect db.Dialect, id uuid.UUID) any {
	if dialect == db.DialectPostgres {
		return id
	}
	return id.String()
}

func scanUUID(dialect db.Dialect, raw any) (uuid.UUID, error) {
	switch value := raw.(type) {
	case uuid.UUID:
		return value, nil
	case string:
		return uuid.Parse(value)
	case []byte:
		return uuid.ParseBytes(value)
	default:
		return uuid.Nil, fmt.Errorf("unsupported uuid type %T for dialect %q", raw, dialect)
	}
}
