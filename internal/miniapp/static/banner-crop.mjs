// Crops are source coordinates in percent, independent of the layout box.
const clamp = (value, min, max) => Math.max(min, Math.min(max, value));

export function defaultSourceCrop(mediaWidth, mediaHeight, aspect) {
	const sourceAspect = Math.max(1, mediaWidth) / Math.max(1, mediaHeight);
	const frameAspect = Math.max(.001, aspect || 2.75);
	const width = sourceAspect > frameAspect ? frameAspect / sourceAspect * 100 : 100;
	const height = sourceAspect > frameAspect ? 100 : sourceAspect / frameAspect * 100;
	return { left: (100 - width) / 2, top: (100 - height) / 2, width, height };
}

export function legacySourceCrop(item, mediaWidth, mediaHeight, aspect) {
	const base = defaultSourceCrop(mediaWidth, mediaHeight, aspect);
	const zoom = clamp(Number(item?.bannerZoom || 100) / 100, 1, 2.5);
	const width = base.width / zoom;
	const height = base.height / zoom;
	// Equivalent to cover + object-position + scale around that same position.
	return {
		left: (100 - width) * clamp(Number(item?.bannerCropX ?? 50), 0, 100) / 100,
		top: (100 - height) * clamp(Number(item?.bannerCropY ?? 50), 0, 100) / 100,
		width, height,
	};
}

export function cropMediaGeometry(crop, sourceWidth, sourceHeight, bounds) {
	const scale = Math.min(bounds.width / (sourceWidth * crop.width / 100), bounds.height / (sourceHeight * crop.height / 100));
	const width = sourceWidth * scale;
	const height = sourceHeight * scale;
	const viewport = {
		width: width * crop.width / 100,
		height: height * crop.height / 100,
	};
	viewport.left = bounds.left + (bounds.width - viewport.width) / 2;
	viewport.top = bounds.top + (bounds.height - viewport.height) / 2;
	return { width, height, left: viewport.left - width * crop.left / 100, top: viewport.top - height * crop.top / 100, viewport };
}

export function zoomCrop(rect, factor) {
	const scale = clamp(1 / Math.max(.1, factor), Math.max(1 / rect.width, 1 / rect.height), Math.min(100 / rect.width, 100 / rect.height));
	const width = rect.width * scale;
	const height = rect.height * scale;
	return {
		left: clamp(rect.left + (rect.width - width) / 2, 0, 100 - width),
		top: clamp(rect.top + (rect.height - height) / 2, 0, 100 - height),
		width, height,
	};
}

export function resizeCropCorner(rect, corner, dx, dy, minWidth = 1, minHeight = 1) {
	const right = rect.left + rect.width;
	const bottom = rect.top + rect.height;
	minWidth = Math.min(Math.max(1, minWidth), rect.width);
	minHeight = Math.min(Math.max(1, minHeight), rect.height);
	const left = corner.endsWith("l") ? clamp(rect.left + dx, 0, right - minWidth) : rect.left;
	const top = corner.startsWith("t") ? clamp(rect.top + dy, 0, bottom - minHeight) : rect.top;
	return {
		left, top,
		width: (corner.endsWith("r") ? clamp(right + dx, left + minWidth, 100) : right) - left,
		height: (corner.startsWith("b") ? clamp(bottom + dy, top + minHeight, 100) : bottom) - top,
	};
}

export function resizeBannerProportionally(width, height, dx, dy, minWidth, minHeight, maxWidth, maxHeight) {
	// Project the pointer onto the original diagonal to retain the aspect ratio.
	const requested = 1 + (dx * width + dy * height) / (width * width + height * height);
	const maximum = Math.min(maxWidth / width, maxHeight / height);
	const minimum = Math.min(maximum, Math.max(minWidth / width, minHeight / height));
	const scale = clamp(requested, minimum, maximum);
	return { width: width * scale, height: height * scale };
}
