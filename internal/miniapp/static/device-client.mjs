export function deviceClientName(userAgent) {
  const value = String(userAgent || "").trim();
  if (!value) return "";
  const known = value.match(/\b(Happ|v2rayTun|Hiddify|INCY|Streisand|Shadowrocket|NekoBox|NekoRay|sing-box|Clash(?:\s+Verge)?|Mihomo|V2Box|FoXray|Karing)\b/i);
  return (known?.[1] || value.split(/[\/\s]/, 1)[0]).slice(0, 40);
}
