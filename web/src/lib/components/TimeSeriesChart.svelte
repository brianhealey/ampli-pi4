<script lang="ts">
	import { formatClockTime } from '$lib/format';

	/** One plotted line. `slot` selects a validated categorical colour. */
	export interface ChartSeries {
		label: string;
		slot: 1 | 2;
		values: number[];
	}

	interface Props {
		title: string;
		series: ChartSeries[];
		/** Epoch milliseconds, one per sample, parallel to every series' values. */
		timestamps: number[];
		unit?: string;
		/** Fixed y-domain. Omit `yMax` to scale to the data. */
		yMin?: number;
		yMax?: number;
		height?: number;
	}

	let {
		title,
		series,
		timestamps,
		unit = '',
		yMin = 0,
		yMax,
		height = 180
	}: Props = $props();

	const pad = { l: 46, r: 14, t: 12, b: 26 };

	let width = $state(640);
	let hoverIndex = $state<number | null>(null);

	const count = $derived(timestamps.length);
	const plotW = $derived(Math.max(1, width - pad.l - pad.r));
	const plotH = $derived(Math.max(1, height - pad.t - pad.b));

	// A fixed domain keeps the line from rescaling on every poll; an auto
	// domain is rounded up so the top gridline stays a round number.
	const domain = $derived.by(() => {
		const lo = yMin;
		if (yMax !== undefined) return { lo, hi: yMax };

		const values = series.flatMap((s) => s.values).filter((v) => Number.isFinite(v));
		const peak = values.length > 0 ? Math.max(...values) : 0;
		const step = peak > 100 ? 20 : 10;
		return { lo, hi: Math.max(lo + step, Math.ceil((peak * 1.15) / step) * step) };
	});

	function toX(index: number): number {
		if (count <= 1) return pad.l + plotW;
		return pad.l + (index / (count - 1)) * plotW;
	}

	function toY(value: number): number {
		const span = domain.hi - domain.lo || 1;
		const clamped = Math.min(Math.max(value, domain.lo), domain.hi);
		return pad.t + plotH - ((clamped - domain.lo) / span) * plotH;
	}

	/** Builds the stroked path; non-finite samples break the line. */
	function linePath(values: number[]): string {
		let path = '';
		let pendingMove = true;
		for (let i = 0; i < values.length; i++) {
			if (!Number.isFinite(values[i])) {
				pendingMove = true;
				continue;
			}
			path += `${pendingMove ? 'M' : 'L'}${toX(i).toFixed(2)},${toY(values[i]).toFixed(2)}`;
			pendingMove = false;
		}
		return path;
	}

	/** Builds a filled area by closing the line down to the baseline. */
	function areaPath(values: number[]): string {
		const line = linePath(values);
		if (!line) return '';
		const baseline = toY(domain.lo).toFixed(2);
		return `${line}L${toX(values.length - 1).toFixed(2)},${baseline}L${toX(0).toFixed(2)},${baseline}Z`;
	}

	const yTicks = $derived([domain.lo, (domain.lo + domain.hi) / 2, domain.hi]);

	// Three time labels — first, middle, last — is enough orientation without
	// crowding the axis. Deduplicated because the midpoint coincides with an
	// end when only two or three samples exist.
	const xTicks = $derived(
		count < 2 ? [] : [...new Set([0, Math.floor((count - 1) / 2), count - 1])]
	);

	const activeIndex = $derived(hoverIndex !== null && hoverIndex < count ? hoverIndex : null);

	function handlePointer(event: PointerEvent) {
		const rect = (event.currentTarget as SVGRectElement).getBoundingClientRect();
		const ratio = (event.clientX - rect.left) / Math.max(1, rect.width);
		hoverIndex = Math.min(count - 1, Math.max(0, Math.round(ratio * (count - 1))));
	}

	function formatValue(value: number): string {
		if (!Number.isFinite(value)) return '—';
		return `${value.toFixed(1)}${unit}`;
	}

	/** Keeps the tooltip inside the plot when hovering near the right edge. */
	const tooltipStyle = $derived.by(() => {
		if (activeIndex === null) return '';
		const x = toX(activeIndex);
		const flip = x > pad.l + plotW * 0.65;
		return flip
			? `right: ${(width - x + 10).toFixed(1)}px; top: ${pad.t}px;`
			: `left: ${(x + 10).toFixed(1)}px; top: ${pad.t}px;`;
	});
</script>

<figure class="chart" bind:clientWidth={width}>
	<figcaption class="chart-header">
		<h3 class="chart-title">{title}</h3>
		{#if series.length > 1}
			<!-- Identity is never colour-alone: the swatch is paired with its label. -->
			<ul class="legend">
				{#each series as s (s.label)}
					<li class="legend-item">
						<span class="swatch" style:background="var(--series-{s.slot})"></span>
						<span>{s.label}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</figcaption>

	<div class="plot-wrap">
		<svg
			viewBox="0 0 {width} {height}"
			{height}
			width="100%"
			role="img"
			aria-label="{title} over time"
		>
			<!-- Recessive gridlines and axis labels -->
			{#each yTicks as tick, i (i)}
				<line
					class="grid"
					x1={pad.l}
					x2={pad.l + plotW}
					y1={toY(tick)}
					y2={toY(tick)}
				/>
				<text class="axis-label" x={pad.l - 8} y={toY(tick)} text-anchor="end" dominant-baseline="middle">
					{Math.round(tick)}{unit}
				</text>
			{/each}

			{#each xTicks as tick (tick)}
				<text
					class="axis-label"
					x={toX(tick)}
					y={height - 8}
					text-anchor={tick === 0 ? 'start' : tick === count - 1 ? 'end' : 'middle'}
				>
					{formatClockTime(new Date(timestamps[tick]).toISOString())}
				</text>
			{/each}

			{#each series as s (s.label)}
				<path class="area" d={areaPath(s.values)} fill="var(--series-{s.slot})" />
			{/each}

			{#each series as s (s.label)}
				<path
					class="line"
					d={linePath(s.values)}
					stroke="var(--series-{s.slot})"
				/>
			{/each}

			{#if activeIndex !== null}
				<line
					class="crosshair"
					x1={toX(activeIndex)}
					x2={toX(activeIndex)}
					y1={pad.t}
					y2={pad.t + plotH}
				/>
				{#each series as s (s.label)}
					{#if Number.isFinite(s.values[activeIndex])}
						<!-- 8px marker with a 2px surface ring so it reads over the line -->
						<circle
							class="marker"
							cx={toX(activeIndex)}
							cy={toY(s.values[activeIndex])}
							r="4"
							fill="var(--series-{s.slot})"
						/>
					{/if}
				{/each}
			{/if}

			<!-- Full-height hit area: easier to target than the marks themselves.
			     Purely a pointer affordance — the table below carries the data
			     for non-visual and keyboard users. -->
			<rect
				role="presentation"
				x={pad.l}
				y={pad.t}
				width={plotW}
				height={plotH}
				fill="transparent"
				onpointermove={handlePointer}
				onpointerleave={() => (hoverIndex = null)}
			/>
		</svg>

		{#if activeIndex !== null}
			<div class="tooltip" style={tooltipStyle}>
				<div class="tooltip-time">
					{formatClockTime(new Date(timestamps[activeIndex]).toISOString())}
				</div>
				{#each series as s (s.label)}
					<div class="tooltip-row">
						<span class="swatch" style:background="var(--series-{s.slot})"></span>
						<span class="tooltip-label">{s.label}</span>
						<span class="tooltip-value">{formatValue(s.values[activeIndex])}</span>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Non-visual route to the same numbers -->
	<details class="data-table">
		<summary>Show data</summary>
		<div class="table-scroll">
			<table>
				<thead>
					<tr>
						<th scope="col">Time</th>
						{#each series as s (s.label)}
							<th scope="col">{s.label}</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each timestamps.slice(-20) as t, i (i)}
						{@const index = Math.max(0, timestamps.length - 20) + i}
						<tr>
							<td>{formatClockTime(new Date(t).toISOString())}</td>
							{#each series as s (s.label)}
								<td>{formatValue(s.values[index])}</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</details>
</figure>

<style>
	.chart {
		/* Validated categorical slots 1-2 (blue, orange) against the card surface. */
		--series-1: #2a78d6;
		--series-2: #eb6834;
		--grid-line: rgba(107, 114, 128, 0.22);
		--crosshair-line: rgba(107, 114, 128, 0.55);
		--text-secondary: #52514e;
		--surface: #ffffff;
		margin: 0;
	}

	@media (prefers-color-scheme: dark) {
		.chart {
			--series-1: #3987e5;
			--series-2: #d95926;
			--grid-line: rgba(156, 163, 175, 0.2);
			--crosshair-line: rgba(156, 163, 175, 0.5);
			--text-secondary: #c3c2b7;
			--surface: #1f2937;
		}
	}

	.chart-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 0.25rem;
	}

	.chart-title {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--text-secondary);
		margin: 0;
	}

	.legend {
		display: flex;
		gap: 0.875rem;
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.legend-item {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.75rem;
		color: var(--text-secondary);
	}

	.swatch {
		width: 10px;
		height: 10px;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.plot-wrap {
		position: relative;
	}

	svg {
		display: block;
		overflow: visible;
		touch-action: none;
	}

	.grid {
		stroke: var(--grid-line);
		stroke-width: 1;
	}

	.axis-label {
		fill: var(--text-secondary);
		font-size: 10px;
		opacity: 0.8;
	}

	.area {
		opacity: 0.12;
	}

	.line {
		fill: none;
		stroke-width: 2;
		stroke-linejoin: round;
		stroke-linecap: round;
	}

	.crosshair {
		stroke: var(--crosshair-line);
		stroke-width: 1;
		stroke-dasharray: 3 3;
	}

	.marker {
		stroke: var(--surface);
		stroke-width: 2;
	}

	.tooltip {
		position: absolute;
		pointer-events: none;
		background: var(--surface);
		border: 1px solid var(--grid-line);
		border-radius: 0.5rem;
		padding: 0.375rem 0.5rem;
		font-size: 0.75rem;
		color: var(--text-secondary);
		box-shadow: 0 4px 12px rgb(0 0 0 / 0.12);
		white-space: nowrap;
		z-index: 10;
	}

	.tooltip-time {
		font-weight: 600;
		margin-bottom: 0.25rem;
	}

	.tooltip-row {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.tooltip-label {
		flex: 1;
	}

	.tooltip-value {
		font-variant-numeric: tabular-nums;
		font-weight: 600;
	}

	.data-table {
		margin-top: 0.5rem;
		font-size: 0.75rem;
		color: var(--text-secondary);
	}

	.data-table summary {
		cursor: pointer;
		opacity: 0.75;
	}

	.table-scroll {
		max-height: 12rem;
		overflow: auto;
		margin-top: 0.375rem;
	}

	.data-table table {
		width: 100%;
		border-collapse: collapse;
		font-variant-numeric: tabular-nums;
	}

	.data-table th,
	.data-table td {
		text-align: left;
		padding: 0.125rem 0.5rem 0.125rem 0;
		white-space: nowrap;
	}

	.data-table th {
		font-weight: 600;
		position: sticky;
		top: 0;
		background: var(--surface);
	}
</style>
