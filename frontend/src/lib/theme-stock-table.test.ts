import { describe, expect, it } from 'vitest';
import type { KLine, SectorMap } from './backend';
import { buildThemeStocks } from './short-term';
import { getThemeStockPage, themeStockPageNumbers, type ThemeStockSort, type ThemeStockTableOptions } from './theme-stock-table';

const meta = { source: 'test', fetched_at: '', latency_ms: 0, stale: false };
const defaults: ThemeStockTableOptions = { sort: 'leader_score', direction: 'desc', page: 1, pageSize: 20, node: 'all', lane: 'all', query: '' };
function fixture(count = 29) {
	const members = Array.from({ length: count }, (_, i) => ({ symbol: `${600000 + i}.SH`, name: `个股${i}`, price: 10, change: 0, change_percent: i - 12, volume: 1, amount: i * 100, limit_up_streak: i, total_market_cap: 0, float_market_cap: 0, main_net_inflow: 0, meta }));
	const map: SectorMap = { theme: 'test', name: '测试', tabs: [], meta, groups: [{ id: 'g', name: '核心', nodes: [
		{ id: 'core', name: '核心细分', change_percent: 0, main_net_inflow: 0, match_status: 'matched', stocks: members },
		{ id: 'branch', name: '目标细分', change_percent: 0, main_net_inflow: 0, match_status: 'matched', stocks: members.slice(20) },
	] }] };
	const order = members.map(stock => stock.symbol);
	const stocks = buildThemeStocks(map).map(stock => {
		const i = order.indexOf(stock.symbol);
		return { ...stock, leader_score: i, tradability_score: count - i, metrics: { ...stock.metrics, return_5d: i - 12 }, limit_regime: i % 2 ? '20cm' as const : '10cm' as const };
	});
	const histories = Object.fromEntries(order.map(symbol => [symbol, [{ symbol, time: '2026-10-12', open: 10, close: 10, high: 10, low: 10, volume: 1, amount: 1, meta } satisfies KLine]]));
	const page = (options: Partial<ThemeStockTableOptions> = {}) => getThemeStockPage(stocks, map, order, histories, { ...defaults, ...options });
	return { stocks, map, order, histories, page };
}

describe('theme stock full-pool sorting and pagination', () => {
	it('sorts the entire pool before pagination, including stocks formerly on later pages', () => {
		const { page, order } = fixture();
		expect(page().stocks.map(stock => stock.symbol)).toEqual([...order].reverse().slice(0, 20));
		expect(page({ page: 2 }).stocks.map(stock => stock.symbol)).toEqual([...order].reverse().slice(20));
		expect(page({ page: 2 }).pagination).toEqual({ page: 2, page_size: 20, total: 29, total_pages: 2, has_more: false });
		expect(page({ direction: 'asc', pageSize: 50 }).stocks.map(stock => stock.symbol)).toEqual(order);
		expect(page({ pageSize: 10, page: 3 }).stocks).toHaveLength(9);
	});

	it.each<ThemeStockSort>(['leader_score', 'tradability_score', 'return_5d', 'change_percent'])('handles %s in both directions, retaining zero and negatives and putting missing values last', sort => {
		const { stocks, histories, page } = fixture(4);
		for (const stock of stocks) {
			const value = [-2, 0, 3, 100][Number(stock.name.slice(2))];
			if (sort === 'return_5d') stock.metrics.return_5d = value;
			else stock[sort] = value;
			if (stock.name === '个股3') { delete histories[stock.symbol]; stock.price = 0; }
		}
		expect(page({ sort, direction: 'asc' }).stocks.map(stock => stock.name)).toEqual(['个股0', '个股1', '个股2', '个股3']);
		expect(page({ sort, direction: 'desc' }).stocks.map(stock => stock.name)).toEqual(['个股2', '个股1', '个股0', '个股3']);
	});

	it('uses source order for ties and excludes retained selections outside the current pool', () => {
		const { stocks, order, map, histories } = fixture(4);
		stocks.forEach(stock => { stock.leader_score = 10; });
		const result = getThemeStockPage(stocks.reverse(), map, order.slice(0, 3), histories, defaults);
		expect(result.stocks.map(stock => stock.symbol)).toEqual(order.slice(0, 3));
	});

	it('filters the full pool by node ID, node name, regime and search before counting or clamping pages', () => {
		const { page } = fixture();
		const result = page({ node: 'branch', lane: '20cm', query: '  目标细分  ', page: 9 });
		expect(result.stocks.map(stock => stock.name)).toEqual(['个股27', '个股25', '个股23', '个股21']);
		expect(result.pagination).toMatchObject({ page: 1, total: 4, total_pages: 1 });
		expect(page({ node: '目标细分' }).pagination.total).toBe(9);
		expect(page({ query: '600024.sh' }).stocks[0].name).toBe('个股24');
		expect(page({ query: '无匹配', page: 3 }).pagination).toMatchObject({ page: 1, total: 0, total_pages: 0, has_more: false });
	});

	it.each<ThemeStockSort>(['amount', 'limit_up_streak'])('preserves the %s sorting option', sort => {
		const { page, order } = fixture();
		expect(page({ sort }).stocks[0].symbol).toBe(order.at(-1));
	});

	it('bounds numbered controls for large pools while retaining first, current and last pages', () => {
		expect(themeStockPageNumbers(1, 1)).toEqual([1]);
		expect(themeStockPageNumbers(2, 3)).toEqual([1, 2, 3]);
		expect(themeStockPageNumbers(5, 50)).toEqual([1, 'ellipsis', 4, 5, 6, 'ellipsis', 50]);
		expect(themeStockPageNumbers(50, 50)).toEqual([1, 'ellipsis', 49, 50]);
	});
});
