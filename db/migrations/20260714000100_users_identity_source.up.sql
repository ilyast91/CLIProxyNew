ALTER TABLE users
    ADD COLUMN identity_source text NOT NULL DEFAULT 'ldap';

ALTER TABLE users
    ADD CONSTRAINT users_identity_source_check
        CHECK (identity_source IN ('ldap', 'static')),
    ADD CONSTRAINT users_identity_source_namespace_check
        CHECK (
            (identity_source = 'static' AND username LIKE 'static:%') OR
            (identity_source = 'ldap' AND username NOT LIKE 'static:%')
        );
