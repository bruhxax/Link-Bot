ALTER TABLE purchase ADD COLUMN device_traffic_terms JSONB;

-- Freeze the currently configured terms only. Do not restore previously
-- disabled quotas or modify any existing Remnawave accounts.
UPDATE purchase p SET device_traffic_terms = COALESCE((
 SELECT jsonb_build_object(
  'enabled', COALESCE(plan->'deviceTrafficEnabled','false'::jsonb),
  'enforced', COALESCE(plan->'deviceTrafficEnforced','false'::jsonb),
  'rules', COALESCE(plan->'deviceTrafficRules','[]'::jsonb))
 FROM app_runtime_settings r, jsonb_array_elements(CASE WHEN jsonb_typeof(r.config->'plans')='array' THEN r.config->'plans' ELSE '[]'::jsonb END) plan
 WHERE r.id=1 AND plan->>'id'=p.plan_id LIMIT 1
), '{"enabled":false,"enforced":false,"rules":[]}'::jsonb)
WHERE p.month>0 OR p.days>0;
