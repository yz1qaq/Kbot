#!/usr/bin/env python3
"""一键验证注册、登录、空间查询、工具创建和工具列表；仅依赖 Python 标准库。"""

import argparse
import json
import os
import sys
import uuid
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8090")
    parser.add_argument("--email", default="kbot-smoke@example.com")
    parser.add_argument(
        "--password", default=os.environ.get("KBOT_SMOKE_PASSWORD", "KbotDemo123!")
    )
    args = parser.parse_args()
    args.email = args.email.strip().lower()
    base_url = args.base_url.rstrip("/")

    def request(method, path, payload=None, token=None, workspace_id=None):
        headers = {"Accept": "application/json"}
        data = None
        if payload is not None:
            headers["Content-Type"] = "application/json"
            data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        if token:
            headers["Authorization"] = "Bearer " + token
        if workspace_id:
            headers["X-Workspace-ID"] = workspace_id
        req = Request(base_url + path, data=data, headers=headers, method=method)
        try:
            response = urlopen(req, timeout=10)
        except HTTPError as exc:
            response = exc
        except (URLError, TimeoutError, OSError) as exc:
            raise RuntimeError(
                f"无法连接 {base_url}，请确认服务已启动及端口正确：{exc}"
            ) from exc
        with response:
            status = response.code
            text = response.read().decode("utf-8")
        try:
            body = json.loads(text)
        except json.JSONDecodeError:
            body = text.strip()
        return status, body

    def expect(status, wanted, body, operation):
        if status != wanted:
            detail = f"：{body}" if isinstance(body, str) else ""
            raise RuntimeError(f"{operation}失败，HTTP {status}，预期 {wanted}{detail}")

    # 1. 首次注册；重复运行时允许测试账号已存在。
    status, body = request("POST", "/api/v1/auth/register", {
        "email": args.email, "password": args.password, "name": "脚本测试用户",
    })
    if status == 400 and "email already exists" in str(body):
        print(f"[1/6] 账号已存在，继续登录：{args.email}")
    else:
        expect(status, 201, body, "注册")
        print(f"[1/6] 注册成功：{args.email}")

    # 2、3. 登录并自动取出 token，不在终端打印完整凭据。
    status, body = request("POST", "/api/v1/auth/login", {
        "email": args.email, "password": args.password,
    })
    expect(status, 200, body, "登录")
    print("[2/6] 登录成功")
    token = body.get("token") if isinstance(body, dict) else None
    if not isinstance(token, str) or not token:
        raise RuntimeError("登录响应缺少 token")
    print("[3/6] 已获取 token，后续请求自动携带")

    # 4. 使用该测试账号的第一个工作空间。
    status, workspaces = request("GET", "/api/v1/workspaces", token=token)
    expect(status, 200, workspaces, "查询工作空间")
    if not isinstance(workspaces, list) or not workspaces:
        raise RuntimeError("账号没有工作空间")
    workspace_id = workspaces[0].get("id")
    if not isinstance(workspace_id, str) or not workspace_id:
        raise RuntimeError("工作空间响应缺少 id")
    print(f"[4/6] 工作空间 ID：{workspace_id}")

    # 5. 每次使用新的名称，避免重复运行触发名称归属冲突。
    tool_name = "search_inventory_" + uuid.uuid4().hex[:12]
    status, body = request("POST", "/api/v1/tools", {
        "name": tool_name,
        "source_type": "http",
        "description": "脚本创建的库存查询测试工具",
        "schema_json": json.dumps({
            "type": "object", "properties": {"sku": {"type": "string"}},
            "required": ["sku"],
        }),
        "endpoint_config": json.dumps({"url": "https://example.com/api/inventory"}),
        "auth_config": json.dumps({"headers": {"Authorization": "Bearer demo-key"}}),
        "sensitive": False,
    }, token=token, workspace_id=workspace_id)
    if status == 200 and body == "":
        print("[提示] 创建接口返回空的 HTTP 200；请在 Register 成功后补上 "
              "writeJSON(w, http.StatusCreated, version)。下面通过列表确认是否创建成功。")
    else:
        expect(status, 201, body, "注册工具")
    print(f"[5/6] 已提交工具注册：{tool_name}")

    # 6. 查询列表，确认本次新工具确实存在。
    status, tools = request(
        "GET", "/api/v1/tools", token=token, workspace_id=workspace_id
    )
    expect(status, 200, tools, "查询工具")
    if not isinstance(tools, list):
        raise RuntimeError("工具列表响应不是数组")
    created = next((item for item in tools if item.get("name") == tool_name), None)
    if created is None:
        raise RuntimeError("列表中没有找到本次注册的工具")
    if created.get("workspace_id") != workspace_id or not created.get("has_auth"):
        raise RuntimeError("新工具的工作空间或 has_auth 标记不符合预期")
    if any("auth_config" in item or "authCiphertext" in item for item in tools):
        raise RuntimeError("工具列表包含认证配置字段")
    print(f"[6/6] 查询成功，共 {len(tools)} 个版本；新版本 ID：{created.get('id')}")
    print(json.dumps(tools, ensure_ascii=False, indent=2))
    print("流程完成：本次创建的工具已在列表中确认。")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, ValueError, KeyError, TypeError) as exc:
        print(f"失败：{exc}", file=sys.stderr)
        sys.exit(1)
