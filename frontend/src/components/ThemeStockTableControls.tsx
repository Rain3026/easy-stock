import { ChevronLeft, ChevronRight } from 'lucide-react';
import type { ThemeScreenPagination } from '../lib/backend';
import { themeStockPageNumbers, type SortDirection, type ThemeStockSort } from '../lib/theme-stock-table';
import './theme-stock-table.css';

export function ThemeStockSortHeader({ column, label, sort, direction, onSort }: {
	column: ThemeStockSort; label: string; sort: ThemeStockSort; direction: SortDirection; onSort: (column: ThemeStockSort) => void;
}) {
	const active = sort === column;
	const next = active && direction === 'desc' ? '升序' : '降序';
	return <th scope="col" aria-sort={active ? direction === 'desc' ? 'descending' : 'ascending' : 'none'}>
		<button type="button" className="stock-sort-button" onClick={() => onSort(column)}
			aria-label={`${label}，${active ? `当前${direction === 'desc' ? '降序' : '升序'}，` : ''}点击${next}`}
			title={`${label}：点击按${next === '降序' ? '从大到小' : '从小到大'}排序`}>
			<span>{label}</span><svg viewBox="0 0 12 18" className="stock-sort-arrows" aria-hidden="true">
				<path className={active && direction === 'asc' ? 'active' : ''} d="M6 2 11 7H1Z" />
				<path className={active && direction === 'desc' ? 'active' : ''} d="m6 16-5-5h10Z" />
			</svg>
		</button>
	</th>;
}

export function ThemeStockPagination({ pagination, complete, fetching, visibleCount, pageSize, onPage, onPageSize }: {
	pagination: ThemeScreenPagination; complete: boolean; fetching: boolean; visibleCount: number; pageSize: number;
	onPage: (page: number) => void; onPageSize: (size: number) => void;
}) {
	const disabled = !complete || fetching;
	const { page, total, total_pages: totalPages } = pagination;
	const start = total ? (page - 1) * pageSize + 1 : 0;
	return <div className="stock-pagination stock-pagination-top" aria-label="题材个股分页">
		<span className="stock-result-count" aria-live="polite">{complete ? <>共 <b>{total}</b> 只<span className="stock-visible-range"> · {start}–{Math.min(page * pageSize, total)}</span></> : `已加载 ${visibleCount} 只 · 完整成分${fetching ? '更新中' : '暂不可用'}`}</span>
		<div className="stock-pagination-actions">
			<label className="stock-page-size">每页<select aria-label="每页行数" value={pageSize} onChange={event => onPageSize(Number(event.target.value))}>{[10, 20, 50].map(size => <option value={size} key={size}>{size}</option>)}</select>行</label>
			<nav aria-label="个股页码">
				<button type="button" aria-label="上一页" title="上一页" disabled={disabled || page <= 1} onClick={() => onPage(page - 1)}><ChevronLeft size={15} aria-hidden="true" /></button>
				{themeStockPageNumbers(page, totalPages || 1).map((value, index) => value === 'ellipsis'
					? <span className="stock-page-ellipsis" key={`gap-${index}`} aria-hidden="true">…</span>
					: <button type="button" key={value} aria-label={`第${value}页`} aria-current={value === page ? 'page' : undefined} disabled={disabled} onClick={() => onPage(value)}>{value}</button>)}
				<button type="button" aria-label="下一页" title="下一页" disabled={disabled || !pagination.has_more} onClick={() => onPage(page + 1)}><ChevronRight size={15} aria-hidden="true" /></button>
			</nav>
			<span className="stock-page-position">{complete ? `${page} / ${totalPages || 1} 页` : '--'}</span>
		</div>
	</div>;
}
