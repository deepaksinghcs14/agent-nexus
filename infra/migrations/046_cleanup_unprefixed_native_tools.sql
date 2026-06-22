-- Remove orphan tool records created when http_request, web_search, read_file, write_file
-- were temporarily renamed to drop the native_ prefix. The canonical names in agent_tools
-- references are the native_-prefixed ones; the unprefixed records are never used.
DELETE FROM tools
WHERE workspace_id IS NULL
  AND type = 'native'
  AND name IN ('http_request', 'web_search', 'read_file', 'write_file');
