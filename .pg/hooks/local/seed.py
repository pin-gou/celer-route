#!/usr/bin/env python3
"""
从 fixature 目录读取 fixture JSON，通过 celer-route-api 的 HTTP API 写入数据。

local 环境的 prepare_env 默认启用仪表盘密码保护（admin / CelerRoute-2026），
种子化前先登录管理面建立会话；认证未启用（如 local-pg）时自动跳过登录，
保持开放模式种子化不变。

用法: seed.py <fixature_dir> <port> [host]
环境变量: BIFROST_ADMIN_USERNAME / BIFROST_ADMIN_PASSWORD（默认 admin / CelerRoute-2026，
         与 local prepare.sh 写入的 auth_config 保持一致）
"""
import http.cookiejar
import json
import os
import sys
import time
import urllib.request
import urllib.error

FIXTURE_DIR = sys.argv[1]
PORT = int(sys.argv[2])
HOST = sys.argv[3] if len(sys.argv) > 3 else "localhost"
BASE = f"http://{HOST}:{PORT}"
TIMEOUT = 15  # per-request timeout

# 管理面凭据：与 local prepare.sh 写入 config.json 的 auth_config 保持一致。
ADMIN_USERNAME = os.environ.get("BIFROST_ADMIN_USERNAME", "admin")
ADMIN_PASSWORD = os.environ.get("BIFROST_ADMIN_PASSWORD", "CelerRoute-2026")

# 带 cookie jar 的 opener：登录成功后会话 cookie 自动附加到后续请求，
# 使开启仪表盘密码保护的 prepare_env 也能正常种子化。
_cookie_jar = http.cookiejar.CookieJar()
_OPENER = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(_cookie_jar))


def api(method, path, body=None, timeout=TIMEOUT):
    url = f"{BASE}{path}"
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    try:
        with _OPENER.open(req, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        return e.code, {"error": err_body[:500]}
    except Exception as e:
        return 0, {"error": str(e)[:300]}


def load_fixture(name):
    path = os.path.join(FIXTURE_DIR, name)
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)


def seed_providers():
    data = load_fixture("fixature_providers.json")
    if not data:
        return
    providers = data.get("providers", [])
    for p in providers:
        pname = p.get("name")
        if not pname:
            continue
        payload = {
            "provider": pname,
            "network_config": p.get("network_config"),
            "concurrency_and_buffer_size": p.get("concurrency_and_buffer_size"),
            "proxy_config": p.get("proxy_config"),
            "send_back_raw_request": p.get("send_back_raw_request"),
            "send_back_raw_response": p.get("send_back_raw_response"),
            "store_raw_request_response": p.get("store_raw_request_response"),
            "custom_provider_config": p.get("custom_provider_config"),
            "openai_config": p.get("openai_config"),
        }
        # 移除 None 值
        payload = {k: v for k, v in payload.items() if v is not None}

        status, resp = api("POST", "/api/providers", payload)
        if status in (200, 201):
            print(f"  ✓ provider {pname}")
        elif status == 409:
            print(f"  ~ provider {pname} 已存在")
        else:
            print(f"  ✗ provider {pname}: {status} {resp.get('error', resp)}")


def seed_keys():
    data = load_fixture("fixature_keys.json")
    if not data:
        return
    for k in data:
        provider = k.get("provider")
        if not provider:
            continue
        # 构造 Key payload
        payload = {
            "name": k.get("name"),
            "key_id": k.get("key_id"),
            "value": k.get("value"),
            "weight": k.get("weight", 1),
            "models": k.get("models", ["*"]),
            "blacklisted_models": k.get("blacklisted_models", []),
            "aliases": k.get("aliases"),
            "enabled": k.get("enabled", True),
            "metadata": k.get("metadata"),
        }
        payload = {k: v for k, v in payload.items() if v is not None}
        # 确保 key_id 传递
        if "key_id" in payload:
            payload["id"] = payload.pop("key_id")

        status, resp = api("POST", f"/api/providers/{provider}/keys", payload)
        if status in (200, 201):
            print(f"  ✓ key {k.get('name')} ({provider})")
        elif status == 409:
            print(f"  ~ key {k.get('name')} ({provider}) 已存在")
        elif status == 0:
            print(f"  ? key {k.get('name')} ({provider}): 请求失败 (超时/网络错误)，数据可能已持久化")
        else:
            print(f"  ✗ key {k.get('name')} ({provider}): {status} {resp.get('error', resp)}")


def seed_routing_rules():
    data = load_fixture("fixature_routing_rules.json")
    if not data:
        return
    rules = data.get("rules", [])
    for r in rules:
        payload = {
            "name": r.get("name"),
            "description": r.get("description", ""),
            "enabled": r.get("enabled", True),
            "chain_rule": r.get("chain_rule", False),
            "cel_expression": r.get("cel_expression"),
            "targets": r.get("targets"),
            "fallbacks": r.get("fallbacks"),
            "scope": r.get("scope", "global"),
            "scope_id": r.get("scope_id"),
            "query": r.get("query"),
            "priority": r.get("priority", 0),
        }
        payload = {k: v for k, v in payload.items() if v is not None}

        status, resp = api("POST", "/api/governance/routing-rules", payload)
        if status in (200, 201):
            print(f"  ✓ routing rule {r.get('name')}")
        elif status == 409:
            print(f"  ~ routing rule {r.get('name')} 已存在")
        else:
            print(f"  ✗ routing rule {r.get('name')}: {status} {resp.get('error', resp)}")


def seed_model_configs():
    data = load_fixture("fixature_model_configs.json")
    if not data:
        return
    configs = data.get("model_configs", [])
    for mc in configs:
        rate_limit = mc.get("rate_limit")
        rl_payload = None
        if rate_limit:
            rl_payload = {
                "token_max_limit": rate_limit.get("token_max_limit"),
                "token_reset_duration": rate_limit.get("token_reset_duration"),
                "request_max_limit": rate_limit.get("request_max_limit"),
                "request_reset_duration": rate_limit.get("request_reset_duration"),
            }
            rl_payload = {k: v for k, v in rl_payload.items() if v is not None}
            if not rl_payload:
                rl_payload = None

        payload = {
            "model_name": mc.get("model_name"),
            "provider": mc.get("provider"),
            "scope": mc.get("scope", "global"),
            "scope_id": mc.get("scope_id"),
            "rate_limit": rl_payload,
            "budgets": mc.get("budgets"),
        }
        payload = {k: v for k, v in payload.items() if v is not None}

        status, resp = api("POST", "/api/governance/model-configs", payload)
        if status in (200, 201):
            print(f"  ✓ model config {mc.get('model_name')}")
        elif status == 409:
            print(f"  ~ model config {mc.get('model_name')} 已存在")
        else:
            print(f"  ✗ model config {mc.get('model_name')}: {status} {resp.get('error', resp)}")


def seed_rate_limits():
    rate_limits = load_fixture("fixature_rate_limits.json")
    # rate_limits 是嵌套对象，这里不单独 seed — 由 model_configs 创建时连带创建


def seed_plugins():
    data = load_fixture("fixature_plugins.json")
    if not data:
        return
    plugins = data.get("plugins", [])
    for p in plugins:
        pname = p.get("name")
        if not pname:
            continue
        payload = {
            "name": pname,
            "enabled": p.get("enabled", True),
            "config": p.get("config") or {},
            "placement": p.get("placement"),
            "order": p.get("order"),
        }
        payload = {k: v for k, v in payload.items() if v is not None}

        status, resp = api("POST", "/api/plugins", payload)
        if status in (200, 201):
            print(f"  ✓ plugin {pname}")
        elif status == 409:
            print(f"  ~ plugin {pname} 已存在")
        else:
            print(f"  ✗ plugin {pname}: {status} {resp.get('error', resp)}")


def login():
    """登录管理面，建立管理员会话。

    prepare_env 默认启用仪表盘密码保护，种子化写操作（/api/providers 等）需要
    会话凭证。登录成功返回 True；认证未启用（HTTP 403，如 local-pg）返回 False，
    继续以开放模式种子化。
    认证已启用但登录失败（错误凭据）时直接退出——否则后续写操作将全部 401，
    种子化结束只见一串 ✗，难以定位根因。
    """
    status, resp = api("POST", "/api/session/login", {
        "username": ADMIN_USERNAME,
        "password": ADMIN_PASSWORD,
    })
    if status == 200:
        print(f"  ✓ 仪表盘密码保护已启用，已登录 {ADMIN_USERNAME}")
        return True
    if status == 403:
        print("  - 认证未启用（HTTP 403），继续以开放模式种子化")
        return False
    print(f"  ✗ 管理面登录失败（HTTP {status}）：{resp.get('error', resp)}")
    print(f"  ✗ 请确认 BIFROST_ADMIN_USERNAME / BIFROST_ADMIN_PASSWORD "
          f"与 prepare.sh 写入 config.json 的 auth_config 一致")
    sys.exit(1)


def seed_proxy_config():
    data = load_fixture("fixature_proxy_config.json")
    if data:
        # 纯 GET 可读，写回需要 PUT /api/proxy-config，有实际内容时才写
        # 当前 fixture 中 proxy_config 为默认值，暂不写
        pass


def main():
    print("=== 开始种子化环境 ===")
    start = time.time()

    # 1. 健康检查
    for i in range(3):
        status, _ = api("GET", "/health", timeout=5)
        if status == 200:
            break
        time.sleep(1)
    else:
        print("ERROR: celer-route-api 无法连接")
        sys.exit(1)

    print("celer-route-api 已连接")

    # 1. 管理面登录（仪表盘密码保护）
    print("1. 管理面登录...")
    login()

    # 2. Provider
    print("2. 创建 provider...")
    seed_providers()

    # 3. Keys
    print("3. 创建 API key...")
    seed_keys()

    # 4. Routing rules
    print("4. 创建路由规则...")
    seed_routing_rules()

    # 5. Model configs
    print("5. 创建模型配置...")
    seed_model_configs()

    # 6. Plugins
    print("6. 创建 plugin...")
    seed_plugins()

    elapsed = time.time() - start
    print(f"=== 种子化完成 ({elapsed:.1f}s) ===")


if __name__ == "__main__":
    main()