interface WeightedItem {
	weight: number;
}

export function normalizeWeightsInPlace<T extends WeightedItem>(items: T[]): T[] {
	if (items.length === 0) return items;
	const total = items.reduce((sum, item) => sum + (Number(item.weight) || 0), 0);
	if (total <= 0) {
		const even = 1 / items.length;
		return items.map((item) => ({ ...item, weight: Number(even.toFixed(4)) }));
	}
	let running = 0;
	return items.map((item, idx) => {
		const isLast = idx === items.length - 1;
		const w = (Number(item.weight) || 0) / total;
		const scaled = isLast ? 1 - running : Number(w.toFixed(4));
		running += scaled;
		return { ...item, weight: scaled };
	});
}