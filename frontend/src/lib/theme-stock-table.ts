import type { SectorMap, ThemeScreenLane, ThemeScreenPagination } from './backend';
import type { KLineLookup, ThemeStock } from './short-term';

export type ThemeStockSort = 'leader_score' | 'tradability_score' | 'return_5d' | 'change_percent' | 'amount' | 'limit_up_streak';
export type SortDirection = 'asc' | 'desc';
export type ThemeStockTableOptions = {
	sort: ThemeStockSort;
	direction: SortDirection;
	page: number;
	pageSize: number;
	node: string;
	lane: ThemeScreenLane;
	query: string;
};

export const themeStockSortColumns = [
	{ key: 'leader_score', label: '领导力' },
	{ key: 'tradability_score', label: '可交易' },
	{ key: 'return_5d', label: '近5日' },
	{ key: 'change_percent', label: '当日' },
] as const;

// Match the values rendered in the table. Unavailable metrics stay last in both
// directions; a real zero or negative return must still participate in sorting.
function sortValue(stock: ThemeStock, sort: ThemeStockSort, histories: KLineLookup): number | undefined {
	let value: number | undefined;
	if (sort === 'leader_score' || sort === 'tradability_score' || sort === 'return_5d') {
		if (!histories[stock.symbol]?.length) return undefined;
		value = sort === 'return_5d' ? stock.metrics.return_5d : stock[sort];
	} else if (sort === 'change_percent') {
		value = stock.live || stock.price > 0 ? stock.change_percent : undefined;
	} else {
		value = stock[sort];
	}
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

export function getThemeStockPage(stocks: ThemeStock[], map: SectorMap | null, sourceOrder: string[], histories: KLineLookup, options: ThemeStockTableOptions) {
	const order = new Map(sourceOrder.map((symbol, index) => [symbol, index]));
	const nodeSymbols = options.node === 'all' ? null : new Set(map?.groups.flatMap(group => group.nodes
		.filter(node => node.id === options.node || node.name === options.node)
		.flatMap(node => node.stocks.map(stock => stock.symbol))) || []);
	const query = options.query.trim().toLowerCase();
	const filtered = stocks.filter(stock => order.has(stock.symbol)
		&& (!nodeSymbols || nodeSymbols.has(stock.symbol))
		&& (options.lane === 'all' || stock.limit_regime === options.lane)
		&& (!query || `${stock.symbol} ${stock.name} ${stock.nodes.join(' ')}`.toLowerCase().includes(query)));
	filtered.sort((a, b) => {
		const av = sortValue(a, options.sort, histories), bv = sortValue(b, options.sort, histories);
		if (av === undefined && bv !== undefined) return 1;
		if (bv === undefined && av !== undefined) return -1;
		if (av !== undefined && bv !== undefined && av !== bv) return options.direction === 'asc' ? av - bv : bv - av;
		return order.get(a.symbol)! - order.get(b.symbol)!;
	});
	const pageSize = Math.max(1, Math.floor(options.pageSize));
	const totalPages = Math.ceil(filtered.length / pageSize);
	const page = Math.max(1, Math.min(options.page, totalPages || 1));
	const start = (page - 1) * pageSize;
	const pagination: ThemeScreenPagination = { page, page_size: pageSize, total: filtered.length, total_pages: totalPages, has_more: page < totalPages };
	return { stocks: filtered.slice(start, start + pageSize), pagination };
}

// Bound the number of controls even for a theme containing hundreds of stocks.
export function themeStockPageNumbers(page: number, total: number): Array<number | 'ellipsis'> {
	const pages = new Set([1, Math.max(1, total)]);
	for (let n = Math.max(1, page - 1); n <= Math.min(total, page + 1); n++) pages.add(n);
	const result: Array<number | 'ellipsis'> = [];
	for (const n of [...pages].sort((a, b) => a - b)) {
		const previous = result.at(-1);
		if (typeof previous === 'number' && n > previous + 1) result.push('ellipsis');
		result.push(n);
	}
	return result;
}
