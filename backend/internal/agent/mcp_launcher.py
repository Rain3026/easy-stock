"""App-owned MCP transport/tool adapter. Never sends or converts model requests."""
import os
import sys
import json

spec = json.loads(os.environ.pop(sys.argv[1]))
# Provider credentials belong to the model process, never to tool subprocesses.
for key in list(os.environ):
    if key.startswith(('CODEX_', 'OPENAI_', 'MODEL_API_KEY', 'EASY_STOCK_MCP_', 'EASY_STOCK_MODEL_')):
        os.environ.pop(key, None)
os.environ.update(spec.get('env', {}))

if spec['transport'] == 'stdio':
    if os.name == 'nt':
        # Windows execvpe does not quote arguments like subprocess does. Keep
        # multiline scripts and paths with spaces intact, and inherit the MCP
        # pipes while the launcher waits for the child process to finish.
        import subprocess
        raise SystemExit(subprocess.run([spec['command'], *spec.get('args', [])], env=os.environ).returncode)
    os.execvpe(spec['command'], [spec['command'], *spec.get('args', [])], os.environ)

import anyio
from mcp.server.stdio import stdio_server

async def relay(source, target):
    async for message in source:
        await target.send(message)

async def serve_sse():
    from mcp.client.sse import sse_client
    async with sse_client(spec['url'], headers=spec.get('headers', {}), timeout=spec.get('connect_timeout') or 30) as (remote_read, remote_write):
        async with stdio_server() as (local_read, local_write):
            async with anyio.create_task_group() as group:
                group.start_soon(relay, local_read, remote_write)
                group.start_soon(relay, remote_read, local_write)

async def serve_tools():
    import contextlib
    import uuid
    # Some upstream tool imports print diagnostics; keep stdout pure MCP.
    with contextlib.redirect_stdout(sys.stderr):
        from mcp.server import Server
        from mcp.types import Tool, TextContent, ListToolsResult, CallToolResult
        from tools.registry import registry
        import tools.web_tools
        import tools.code_execution_tool
        import tools.browser_tool
    permitted = set()
    if 'web' in spec['toolsets']:
        permitted.update(['web_search', 'web_extract'])
    if 'code_execution' in spec['toolsets']:
        permitted.add('execute_code')
    if spec.get('browser_state'):
        # Login is performed by the product's browser. This worker reads pages.
        permitted.update(['browser_navigate', 'browser_snapshot', 'browser_get_text', 'browser_scroll', 'browser_close'])
    task_id = 'easy-stock-' + uuid.uuid4().hex

    async def list_tools():
        result = []
        for name in sorted(permitted):
            schema = registry.get_schema(name)
            if name == 'execute_code':
                from tools.code_execution_tool import build_execute_code_schema
                schema = build_execute_code_schema(permitted - {'execute_code'}, mode='strict')
                schema['description'] = '在当前任务的临时工作区执行 Python 计算，输出结果至 stdout。不可访问用户文件、启动命令或直接访问网络；网页访问仅可使用已列出的 web_search/web_extract。'
                schema['parameters']['properties']['code']['description'] = 'Python 计算代码；使用 print 输出结果。'
            if schema:
                result.append(Tool(name=name, description=schema.get('description', ''), inputSchema=schema.get('parameters', {'type': 'object', 'properties': {}})))
        return result

    async def call_tool(name, arguments):
        if name not in permitted:
            raise ValueError('Tool is outside this task policy')
        def run():
            with contextlib.redirect_stdout(sys.stderr):
                return registry.dispatch(name, arguments or {}, task_id=task_id, enabled_tools=sorted(permitted))
        result = await anyio.to_thread.run_sync(run)
        text = result if isinstance(result, str) else json.dumps(result, ensure_ascii=False)
        return [TextContent(type='text', text=text[:100000])]

    if hasattr(Server, 'list_tools'):
        # Hermes releases may bundle either major version of the MCP SDK.
        server = Server('easy-stock-research-tools')
        server.list_tools()(list_tools)
        server.call_tool()(call_tool)
    else:
        async def on_list_tools(ctx, params):
            return ListToolsResult(tools=await list_tools())
        async def on_call_tool(ctx, params):
            return CallToolResult(content=await call_tool(params.name, params.arguments))
        server = Server('easy-stock-research-tools', on_list_tools=on_list_tools, on_call_tool=on_call_tool)

    async with stdio_server() as (read, write):
        await server.run(read, write, server.create_initialization_options())

async def serve_app_data():
    import urllib.request
    from mcp.server import Server
    from mcp.types import Tool, ToolAnnotations, TextContent, ListToolsResult, CallToolResult
    def request(path, data=None):
        raw = None if data is None else json.dumps(data, ensure_ascii=False).encode()
        req = urllib.request.Request(spec['url'] + path, data=raw, headers={
            'Authorization': 'Bearer ' + spec['token'], 'Content-Type': 'application/json'})
        # Loopback task scope; never inherit proxy routing or user credentials.
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(req, timeout=42) as response:
            return json.load(response)
    async def list_tools():
        rows = await anyio.to_thread.run_sync(lambda: request('/tools'))
        return [Tool(name=r['name'], description=r['description'], inputSchema=r['inputSchema'], annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False, idempotentHint=True, openWorldHint=False)) for r in rows]
    async def call_tool(name, arguments):
        result = await anyio.to_thread.run_sync(lambda: request('/call', {'name': name, 'arguments': arguments or {}}))
        return CallToolResult(content=[TextContent(type='text', text=json.dumps(result, ensure_ascii=False))], isError=bool(result.get('isError')))
    if hasattr(Server, 'list_tools'):
        server = Server('easy-stock-data')
        server.list_tools()(list_tools)
        server.call_tool()(call_tool)
    else:
        async def on_list(ctx, params):
            return ListToolsResult(tools=await list_tools())
        async def on_call(ctx, params):
            return await call_tool(params.name, params.arguments)
        server = Server('easy-stock-data', on_list_tools=on_list, on_call_tool=on_call)
    async with stdio_server() as (read, write):
        await server.run(read, write, server.create_initialization_options())

anyio.run(serve_app_data if spec['transport'] == 'app_data' else serve_sse if spec['transport'] == 'sse' else serve_tools)
