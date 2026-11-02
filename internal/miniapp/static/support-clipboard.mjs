export function isSupportedSupportMedia(file) {
	const type = String(file?.type || "").toLowerCase();
	const name = String(file?.name || "").toLowerCase();
	return /^(?:image\/(?:jpeg|png|webp|gif)|video\/(?:mp4|webm|quicktime))$/.test(type) || /\.(?:jpe?g|png|webp|gif|mp4|webm|mov)$/.test(name);
}

export function supportMediaFromClipboard(clipboardData) {
	const files = Array.from(clipboardData?.files || []);
	for (const item of Array.from(clipboardData?.items || [])) {
		if (item.kind !== "file") continue;
		const file = item.getAsFile?.();
		if (file && !files.includes(file)) files.push(file);
	}
	return files.find(isSupportedSupportMedia) || null;
}
