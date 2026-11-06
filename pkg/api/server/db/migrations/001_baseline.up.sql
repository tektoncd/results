-- Baseline Results schema. Identical to the schema created by GORM
-- AutoMigrate for pkg/api/server/db.Result and Record, so databases created by
-- earlier releases can be adopted as version 1 without changes.

CREATE TABLE results (
    parent character varying(64) NOT NULL,
    id character varying(64) NOT NULL,
    name character varying(64),
    annotations jsonb,
    created_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    recordsummary_record character varying(256),
    recordsummary_type character varying(768),
    recordsummary_start_time timestamp with time zone,
    recordsummary_end_time timestamp with time zone,
    recordsummary_status integer,
    recordsummary_annotations jsonb,
    etag character varying(128),
    CONSTRAINT results_pkey PRIMARY KEY (parent, id)
);

CREATE UNIQUE INDEX results_by_name ON results USING btree (parent, name);

CREATE TABLE records (
    parent character varying(64) NOT NULL,
    result_id character varying(64) NOT NULL,
    result_name character varying(64),
    id character varying(64) NOT NULL,
    name character varying(64),
    type character varying(768),
    data jsonb,
    created_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_time timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    etag character varying(128),
    CONSTRAINT records_pkey PRIMARY KEY (parent, result_id, id),
    CONSTRAINT fk_records_result FOREIGN KEY (parent, result_id)
        REFERENCES results (parent, id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE UNIQUE INDEX records_by_name ON records USING btree (parent, result_name, name);
