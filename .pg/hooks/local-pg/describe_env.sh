#!/usr/bin/env bash
# local-pg environment describe_env hook: 描述 prepare_env 执行后的预期环境基线状态
# 输出 env-description.yaml 到 stdout，由 pg-run-hook.py 定向到 .pg/changes/<id>/env-description.yaml
set -uo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PG_SKILLS_PATH="${PG_SKILLS_PATH:-$SELF_DIR}"
source "$PG_SKILLS_PATH/src/runtime/lib/hook-helpers.sh"
trap 'pg_fail_on_error $? $LINENO' ERR

CALLER="${PG_RUN_CALLER:-ad-hoc}"
ENV_NAME="${PG_ENV:-local-pg}"
CHANGE="${PG_RUN_SESSION:-}"

# Postgres 参数（与 postgres.sh 默认一致，可 LOCAL_PG_* 覆盖）
PG_HOST="${LOCAL_PG_HOST:-localhost}"
PG_PORT="${LOCAL_PG_PORT:-55432}"
PG_USER="${LOCAL_PG_USER:-celer}"
PG_DB="${LOCAL_PG_DB:-celer_route}"

described_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ -n "${PG_OUTPUT_PATH:-}" ]]; then
    exec >"$PG_OUTPUT_PATH" 2>&1
fi

cat <<YAML
schema_version: 1
described_by: describe_env.sh
described_at: "${described_at}"
described_for:
  caller: ${CALLER}
  environment: ${ENV_NAME}
  $(if [ -n "${CHANGE}" ]; then echo "change: \"${CHANGE}\""; fi)

environments:
  ${ENV_NAME}:
    business_systems:
      - name: celer-route-api
        type: rest-api
        category: upstream
        description: "celer-route AI 网关 HTTP API。config_store 与 logs_store 均使用 Postgres（${PG_HOST}:${PG_PORT}/${PG_DB}）。prepare_env 阶段启动后通过 fixture 数据种子化，然后关闭。"
        endpoints:
          - name: health
            url: "http://localhost:9080/health"
            method: GET
            health_check:
              method: GET
              path: /health
              expect_status: 200
              timeout_seconds: 5
          - name: api
            url: "http://localhost:9080"
            method: POST
        lifecycle: ephemeral
        reachable: false
        tags:
          - bifrost
          - gateway
          - postgres
        capabilities:
          - llm_request_logging
          - rest_api_endpoint
          - sse_streaming
          - bifrost_openai_compatible_api
          - bifrost_build_artifact

      - name: ui-dev
        type: web-app
        category: upstream
        description: "Vite + React 前端开发服务器（NPM run dev，端口 3008）。支持中英文 i18n 运行时切换与 localStorage 持久化（celer-route.locale）。"
        endpoints:
          - name: dev
            url: "http://localhost:3008"
            method: GET
            health_check:
              method: GET
              path: /
              expect_status: 200
              timeout_seconds: 10
        lifecycle: ephemeral
        reachable: false
        tags:
          - vite
          - react
          - i18n
        capabilities:
          - vite_dev_server
          - react_spa
          - i18n_runtime_switch
          - jsdom_runtime

    data_resources:
      - name: postgres-celer-route
        type: database
        owner: celer-route-api
        description: "Postgres 实例（${PG_HOST}:${PG_PORT}，容器 celer-pg-local-pg），库 ${PG_DB}（用户 ${PG_USER}）。同时承载配置存储与日志存储。"
        state:
          status: seeded
        schema_ref: "${PG_HOST}:${PG_PORT}/${PG_DB}"
        lifecycle: managed
        cleanup: drop
        tags:
          - postgres
          - docker
        capabilities:
          - postgres_db
          - config_store
          - logs_store

      - name: config-db
        type: db-table
        owner: celer-route-api
        description: "Postgres 配置数据库 ${PG_DB}，包含 providers、keys、routing_rules、model_configs、plugins 等表（config_store=postgres）"
        state:
          status: seeded
        schema_ref: "postgres:${PG_HOST}:${PG_PORT}/${PG_DB}"
        lifecycle: managed
        cleanup: drop
        tags:
          - postgres
          - config
        capabilities:
          - sample_dataset
          - postgres_config_store

      - name: logs-db
        type: db-table
        owner: celer-route-api
        description: "Postgres 日志存储（logs 表，logs_store=postgres），存储请求/响应日志"
        state:
          status: empty
        schema_ref: "postgres:${PG_HOST}:${PG_PORT}/${PG_DB}"
        lifecycle: managed
        cleanup: drop
        tags:
          - postgres
          - logs
        capabilities:
          - postgres_logs_store
          - llm_request_logging

      - name: fixture-providers
        type: db-row
        owner: config-db
        description: "种子 provider 配置：ali（通义千问）、deepseek、kimi、minimax、sensova"
        state:
          status: seeded
          row_count: 5
        lifecycle: persistent
        cleanup: keep
        tags:
          - fixture
          - provider
        capabilities:
          - sample_dataset

      - name: fixture-keys
        type: db-row
        owner: config-db
        description: "种子 API key 配置，每个 provider 1-3 个 key"
        state:
          status: seeded
          row_count: 8
        lifecycle: persistent
        cleanup: keep
        tags:
          - fixture
          - key
        capabilities:
          - sample_dataset

      - name: fixture-routing-rules
        type: db-row
        owner: config-db
        description: "种子路由规则：pg-expert、pg-master、pg-associate、hermes-default、default"
        state:
          status: seeded
          row_count: 5
        lifecycle: persistent
        cleanup: keep
        tags:
          - fixture
          - routing-rule
        capabilities:
          - sample_dataset

      - name: fixture-model-configs
        type: db-row
        owner: config-db
        description: "种子模型配置，含 rate_limit、budgets 等"
        state:
          status: seeded
          row_count: 5
        lifecycle: persistent
        cleanup: keep
        tags:
          - fixture
          - model-config
        capabilities:
          - sample_dataset

      - name: fixture-plugins
        type: db-row
        owner: config-db
        description: "种子 plugin 配置"
        state:
          status: seeded
          row_count: 2
        lifecycle: persistent
        cleanup: keep
        tags:
          - fixture
          - plugin
        capabilities:
          - sample_dataset

    config_resources:
      - name: fixture-data-dir
        type: yaml-config
        location: .pg/hooks/local-pg/data/
        scope: environment
        description: "local-pg 数据目录，含 config.json（config_store/logs_store 指向 Postgres）"
        lifecycle: persistent
        tags:
          - data
          - postgres

      - name: config-json
        type: json-config
        location: .pg/hooks/local-pg/data/config.json
        scope: environment
        description: "启动配置：config_store 与 logs_store 均为 postgres（host/port/user/password/db_name/ssl_mode=disable）"
        lifecycle: persistent
        tags:
          - config
          - postgres

      - name: fixture-source-dir
        type: file
        location: .pg/hooks/local/fixature/
        scope: environment
        description: "种子数据 JSON 文件源目录（复用 local 环境），共 12 个 fixture 文件"
        lifecycle: persistent
        tags:
          - fixture
          - seed

      - name: bifrost-binary
        type: file
        location: tmp/celer-route-http
        scope: environment
        description: "编译后的 celer-route-http 二进制文件"
        lifecycle: persistent
        tags:
          - binary
          - bifrost
        capabilities:
          - bifrost_build_artifact

    runtime_environment:
      - name: localhost
        type: network
        config:
          host: localhost
          ports:
            - port: 9080
              role: celer-route-api
              description: "celer-route HTTP API 端口（prepare_env 阶段使用后关闭）"
            - port: 3008
              role: ui-dev
              description: "Vite 开发服务器端口（由 stage 的 role start 启动）"
            - port: ${PG_PORT}
              role: postgres
              description: "Postgres 容器映射端口（celer-pg-local-pg）"
        scope: host
        lifecycle: persistent
        tags:
          - local-pg
          - development

    relations:
      - from: config-db
        to: celer-route-api
        type: owns
        criticality: required
        description: "celer-route 从 Postgres ${PG_DB} 读取配置（config_store=postgres）"

      - from: logs-db
        to: celer-route-api
        type: owns
        criticality: required
        description: "celer-route 写入 Postgres ${PG_DB} 存储日志（logs_store=postgres）"

      - from: postgres-celer-route
        to: config-db
        type: hosts
        criticality: required
        description: "Postgres 实例承载配置库"

      - from: postgres-celer-route
        to: logs-db
        type: hosts
        criticality: required
        description: "Postgres 实例承载日志库"

      - from: fixture-providers
        to: config-db
        type: depends_on
        criticality: required
        description: "provider 行数据属于 config-db 的 providers 表"

      - from: fixture-keys
        to: config-db
        type: depends_on
        criticality: required
        description: "key 行数据属于 config-db 的 keys 表"

      - from: fixture-routing-rules
        to: config-db
        type: depends_on
        criticality: required
        description: "路由规则行数据属于 config-db 的 routing_rules 表"

      - from: fixture-model-configs
        to: config-db
        type: depends_on
        criticality: required
        description: "模型配置行数据属于 config-db 的 model_configs 表"

      - from: fixture-plugins
        to: config-db
        type: depends_on
        criticality: required
        description: "plugin 行数据属于 config-db 的 plugins 表"

      - from: fixture-source-dir
        to: config-db
        type: references
        criticality: required
        description: "seed.py 读取 local/fixature 目录的 JSON 文件，经 API 写入 Postgres 配置库"

      - from: config-json
        to: celer-route-api
        type: configures
        criticality: required
        description: "config.json 决定 celer-route 使用 Postgres 作为 config_store 与 logs_store"

      - from: ui-dev
        to: localhost
        type: depends_on
        criticality: required
        description: "ui-dev 通过 localhost:3008 暴露 Vite 开发服务器"

      - from: ui-dev
        to: celer-route-api
        type: consumes
        criticality: required
        description: "ui-dev 通过 http://localhost:9080 调 celer-route REST API 加载数据"
YAML

pg_exit --status=pass --duration=0 \
  --metadata="env=\"${ENV_NAME}\" caller=\"${CALLER}\" pg=\"${PG_HOST}:${PG_PORT}/${PG_DB}\""