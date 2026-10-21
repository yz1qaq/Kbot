-- 000001_init_iam：身份与组织。

CREATE TABLE users (
    id            UUID PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'member',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE TABLE workspaces (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parent_id   TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);


/*
记录哪个用户属于哪个空间，以及他在该空间中的角色。
*/
CREATE TABLE workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'member',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX workspace_members_user ON workspace_members (user_id);


CREATE TABLE roles (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);


/*
某个用户，在某个工作空间，被分配了某个角色定义。
*/
CREATE TABLE user_roles (
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id      UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id, workspace_id)
);


/*
某个角色，可以对某种资源执行某种动作。
*/
CREATE TABLE role_permissions (
    role     TEXT NOT NULL,
    resource TEXT NOT NULL,
    action   TEXT NOT NULL,
    PRIMARY KEY (role, resource, action)
);


/*
保存平台访问凭据的记录。一个用户可以拥有多把 Key，例如给不同程序使用
*/
CREATE TABLE api_keys (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    hash         TEXT UNIQUE NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX api_keys_user ON api_keys (user_id);