-- +goose Up
-- +goose StatementBegin
CREATE TABLE media_marker
(
    id          varchar(255) not null primary key,
    item_id     varchar(255) not null,
    item_type   varchar(255) not null,
    kind        varchar(255) not null,
    start_ms    integer      not null,
    end_ms      integer,
    source      varchar(255) not null,
    confidence  real,
    created_at  datetime,
    updated_at  datetime
);

CREATE INDEX media_marker_item_id ON media_marker (item_id, item_type);
CREATE INDEX media_marker_kind ON media_marker (kind);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE media_marker;
-- +goose StatementEnd
