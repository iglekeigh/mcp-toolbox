# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Script to verify server-side URL Parameter Binding using the Toolbox Python SDK client.

Prerequisites:
1. Start the MCP Toolbox server with the sqlite prebuilt configuration:
   export SQLITE_DATABASE=/tmp/test.db
   go run . --prebuilt sqlite --port 5000

2. Install the Toolbox Python SDK client:
   pip3 install toolbox-core
   (Note: On macOS, if pip3 installs to Python 3.11+, use python3.11 to run this script).
"""

import asyncio
import os
import sys

import urllib.parse

from toolbox_core import ToolboxClient
from toolbox_core.protocol import Protocol


async def verify_url_parameter_binding():
    # 1. Configure the server endpoint and URL query parameters.
    # The sqlite prebuilt configuration exposes an 'execute_sql' tool that requires a 'sql' parameter.
    # By appending '?sql=SELECT+42' to the connection URL, we instruct the server's transport layer
    # to bind this parameter for the duration of the SSE session.
    base_url = os.environ.get("TOOLBOX_URL", "http://127.0.0.1:5000").rstrip("/")
    if not base_url.endswith("/mcp"):
        base_url = f"{base_url}/mcp"
        
    bound_sql_query = "SELECT 42 AS answer; --"
    # Format query parameter for URL robustly encoding semicolons and special characters
    url_param = urllib.parse.quote_plus(bound_sql_query)
    bound_url = f"{base_url}?sql={url_param}"
    
    print("================================================================================")
    print(f"Connecting to Toolbox server at: {bound_url}")
    print("================================================================================\n")

    # 2. Connect to the server using the ToolboxClient context manager.
    # We explicitly specify Protocol.MCP_LATEST to use the latest protocol features and prevent warnings.
    async with ToolboxClient(bound_url, protocol=Protocol.MCP_LATEST) as client:
        print("[STEP 1] Fetching toolset from server (tools/list)...")
        tools = await client.load_toolset()
        
        execute_sql_tool = next((t for t in tools if t.__name__ == "execute_sql"), None)
        if not execute_sql_tool:
            print("[ERROR] 'execute_sql' tool not found in loaded toolset. Ensure server is running with '--prebuilt sqlite'.", file=sys.stderr)
            sys.exit(1)

        print(f"Successfully loaded tool: '{execute_sql_tool.__name__}'")
        print(f"Tool description: {execute_sql_tool.__doc__.strip()}")
        
        # Notice that because 'sql' was passed in the URL, the server's CloneAndFilter logic
        # stripped 'sql' from the tool's required schema properties. E.g. the client does not
        # see 'sql' as a required parameter.

        # 3. Execute the tool without providing the bound parameter.
        print("\n[STEP 2] Invoking execute_sql() with NO arguments...")
        print(f"Expecting server to auto-inject bound query: '{bound_sql_query}'")
        
        try:
            # Calling execute_sql() with empty parameters.
            # The server's toolsCallHandler will intercept the call and inject sql="SELECT 42 AS answer;".
            result = await execute_sql_tool()
            print(f"\n[SUCCESS] Tool execution returned: {result}")
            
            # Verification check
            assert result is not None, "Expected a non-null execution result from server"
            print("\n================================================================================")
            print("URL Parameter Binding verification completed successfully!")
            print("================================================================================")
        
        except Exception as e:
            print(f"\n[FAILURE] Tool execution failed with error: {e}", file=sys.stderr)
            sys.exit(1)


if __name__ == "__main__":
    try:
        asyncio.run(verify_url_parameter_binding())
    except KeyboardInterrupt:
        print("\nVerification cancelled by user.")
