#!/usr/bin/env bash
# Run this once after cloning to scaffold all remaining page stubs.
# Each page just renders a placeholder until properly implemented.

PAGES=(
  "apps/web/src/app/dashboard/page.tsx"
  "apps/web/src/app/agents/page.tsx"
  "apps/web/src/app/agents/new/page.tsx"
  "apps/web/src/app/playground/page.tsx"
  "apps/web/src/app/conversations/page.tsx"
  "apps/web/src/app/runs/page.tsx"
  "apps/web/src/app/memory/page.tsx"
  "apps/web/src/app/usage/page.tsx"
  "apps/web/src/app/tools/page.tsx"
  "apps/web/src/app/mcp-servers/page.tsx"
  "apps/web/src/app/agent-groups/page.tsx"
  "apps/web/src/app/agent-groups/new/page.tsx"
  "apps/web/src/app/settings/providers/page.tsx"
  "apps/web/src/app/settings/workspace/page.tsx"
  "apps/web/src/app/admin/overview/page.tsx"
  "apps/web/src/app/admin/users/page.tsx"
  "apps/web/src/app/admin/workspaces/page.tsx"
  "apps/web/src/app/admin/policies/page.tsx"
  "apps/web/src/app/admin/audit-logs/page.tsx"
)

for page in "${PAGES[@]}"; do
  # Extract route name from path
  name=$(basename "$(dirname "$page")")
  mkdir -p "$(dirname "$page")"
  cat > "$page" <<EOF
export default function ${name^}Page() {
  return (
    <div className="p-6">
      <h1 className="text-lg font-medium text-gray-900">${name^}</h1>
      <p className="text-sm text-gray-500 mt-1">Not implemented yet — Step 15+</p>
    </div>
  )
}
EOF
  echo "Created $page"
done

echo "Done — all page stubs created."
