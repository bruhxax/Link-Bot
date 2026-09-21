const SUPPORT_MESSAGE_CANDIDATE_PATTERN = /(?:https?:\/\/|www\.|(?:vless|vmess|trojan|ss|ssr|hysteria2?|hy2|tuic|wireguard):\/\/)[^\s<>"']+|@[a-z][a-z0-9_]{4,31}/giu;

export function isStandaloneTelegramUsername(source, start, end) {
	const previous = source.slice(Math.max(0, start - 1), start);
	const next = source.slice(end, end + 1);
	return !/[\p{L}\p{N}_.+-]/u.test(previous) && !/[A-Za-z0-9_]/.test(next);
}

export function tokenizeSupportMessage(body) {
	const source = String(body || "");
	const tokens = [];
	let offset = 0;
	for (const match of source.matchAll(SUPPORT_MESSAGE_CANDIDATE_PATTERN)) {
		const value = match[0];
		const start = Number(match.index || 0);
		const end = start + value.length;
		const username = value.startsWith("@");
		if (username && !isStandaloneTelegramUsername(source, start, end)) continue;
		if (start > offset) tokens.push({ type: "text", value: source.slice(offset, start) });
		tokens.push({ type: username ? "username" : "link", value });
		offset = end;
	}
	if (offset < source.length) tokens.push({ type: "text", value: source.slice(offset) });
	return tokens;
}
