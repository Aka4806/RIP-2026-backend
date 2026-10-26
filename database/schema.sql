CREATE TABLE users (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username varchar(100) NOT NULL UNIQUE,
    password varchar(255) NOT NULL UNIQUE CHECK (length(password) > 0)
);

CREATE TABLE index_configurations (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name varchar(150) NOT NULL,
    short_description varchar(500),
    status varchar(20) NOT NULL CHECK (status IN ('draft', 'published', 'deleted')),
    image_url varchar(500),
    video_url varchar(500),
    table_size integer NOT NULL,
    selectivity numeric(10,6) NOT NULL,
    created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
    creator_id integer NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    formed_at timestamp
);

CREATE UNIQUE INDEX uq_one_draft_per_user ON index_configurations (creator_id) WHERE status = 'draft';

CREATE TABLE index_configuration_likes (
    id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    index_configuration_id integer NOT NULL REFERENCES index_configurations(id) ON DELETE RESTRICT,
    CONSTRAINT uq_user_index_like UNIQUE (user_id, index_configuration_id)
);
