import { type CSSProperties, type SVGProps, useLayoutEffect, useRef, useState } from 'react';

/** Keep chart labels readable when a viewBox shrinks to fit its container. */
export function ChartSvg({ style, viewBox, ...props }: Omit<SVGProps<SVGSVGElement>, 'ref'>) {
	const ref = useRef<SVGSVGElement>(null);
	const [textScale, setTextScale] = useState(1);

	useLayoutEffect(() => {
		const chart = ref.current;
		if (!chart) return;
		const updateScale = () => {
			const matrix = chart.getScreenCTM();
			if (!matrix) return;
			const scale = Math.hypot(matrix.a, matrix.b);
			if (scale <= 0) return;
			const next = Math.max(1, 1 / scale);
			setTextScale((current) => Math.abs(current - next) < .0001 ? current : next);
		};
		updateScale();
		const observer = new ResizeObserver(updateScale);
		observer.observe(chart);
		return () => observer.disconnect();
	}, [viewBox]);

	return <svg {...props} ref={ref} viewBox={viewBox} data-chart-font-size="" style={{ ...style, '--chart-text-scale': textScale } as CSSProperties} />;
}
