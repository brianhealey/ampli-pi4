<script lang="ts">
	import { onMount } from 'svelte';
	import { diagnostics } from '$lib/diagnosticsStore.svelte';
	import { amplipi } from '$lib/store.svelte';
	import TimeSeriesChart from '$lib/components/TimeSeriesChart.svelte';
	import type { ChartSeries } from '$lib/components/TimeSeriesChart.svelte';
	import StatTile from '$lib/components/StatTile.svelte';
	import UsageMeter from '$lib/components/UsageMeter.svelte';
	import {
		formatBytes,
		formatDateTime,
		formatDuration,
		formatFrequency,
		formatPercent,
		formatTemp
	} from '$lib/format';

	onMount(() => {
		diagnostics.start();
		return () => diagnostics.stop();
	});

	const stats = $derived(diagnostics.stats);
	const hardware = $derived(diagnostics.hardware);
	const timestamps = $derived(diagnostics.history.map((p) => p.t));

	// CPU and memory share a 0-100% axis, so they belong on one chart.
	// Temperature has different units and gets its own — never a second y-axis.
	const utilizationSeries = $derived<ChartSeries[]>([
		{ label: 'CPU', slot: 1, values: diagnostics.history.map((p) => p.cpu) },
		{ label: 'Memory', slot: 2, values: diagnostics.history.map((p) => p.memory) }
	]);

	const temperatureSeries = $derived<ChartSeries[]>([
		{ label: 'CPU temperature', slot: 1, values: diagnostics.history.map((p) => p.temp) }
	]);

	/** Throttling conditions worth surfacing, with plain-language wording. */
	const throttleWarnings = $derived.by(() => {
		const t = stats?.thermal;
		if (!t) return [];
		const warnings: string[] = [];
		if (t.under_voltage) warnings.push('Under-voltage detected now — check the power supply');
		if (t.throttled) warnings.push('CPU is being throttled now');
		if (t.freq_capped) warnings.push('CPU frequency is capped now');
		if (t.soft_temp_limit) warnings.push('Soft temperature limit reached now');
		if (!t.under_voltage && t.under_volt_ever) warnings.push('Under-voltage has occurred since boot');
		if (!t.throttled && t.throttled_ever) warnings.push('Throttling has occurred since boot');
		return warnings;
	});

	const availableStreams = $derived(hardware?.stream_support.filter((s) => s.available) ?? []);
	const missingStreams = $derived(hardware?.stream_support.filter((s) => !s.available) ?? []);
</script>

<svelte:head>
	<title>About — AmpliPi</title>
</svelte:head>

<div class="space-y-6 p-4 md:p-6">
	<header>
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">About this device</h1>
		<p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
			{stats?.host.hostname ?? '—'}
			{#if stats?.host.model}· {stats.host.model}{/if}
			{#if diagnostics.version}· AmpliPi {diagnostics.version}{/if}
		</p>
	</header>

	{#if diagnostics.error}
		<div class="rounded-lg bg-red-50 p-4 dark:bg-red-900/20">
			<h2 class="font-semibold text-red-800 dark:text-red-200">Diagnostics unavailable</h2>
			<p class="text-sm text-red-600 dark:text-red-400">{diagnostics.error}</p>
		</div>
	{/if}

	{#if throttleWarnings.length > 0}
		<div class="rounded-lg border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20">
			<h2 class="flex items-center gap-2 font-semibold text-amber-900 dark:text-amber-200">
				<span aria-hidden="true">⚠️</span> Power &amp; thermal warnings
			</h2>
			<ul class="mt-2 list-inside list-disc space-y-1 text-sm text-amber-800 dark:text-amber-300">
				{#each throttleWarnings as warning (warning)}
					<li>{warning}</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if diagnostics.loading && !stats}
		<p class="text-sm text-gray-500 dark:text-gray-400">Loading device statistics…</p>
	{:else if stats}
		<!-- Headline numbers -->
		<section class="grid grid-cols-2 gap-3 lg:grid-cols-5">
			<StatTile
				icon="⚡"
				label="CPU usage"
				value={formatPercent(stats.cpu.usage_percent, 1)}
				detail="{stats.cpu.cores} cores · load {stats.cpu.load_avg_1.toFixed(2)}"
			/>
			<StatTile
				icon="🧠"
				label="Memory"
				value={formatPercent(stats.memory.used_percent, 1)}
				detail="{formatBytes(stats.memory.used_bytes)} of {formatBytes(stats.memory.total_bytes)}"
			/>
			<StatTile
				icon="🌡️"
				label="CPU temp"
				value={stats.thermal.cpu_temp_c > 0 ? formatTemp(stats.thermal.cpu_temp_c) : '—'}
				detail={stats.cpu.frequency_mhz > 0 ? formatFrequency(stats.cpu.frequency_mhz) : ''}
			/>
			<StatTile
				icon="⏱️"
				label="Uptime"
				value={formatDuration(stats.host.uptime_seconds)}
				detail={stats.host.boot_time ? `Booted ${formatDateTime(stats.host.boot_time)}` : ''}
			/>
			<StatTile
				icon="🎛️"
				label="Controllers"
				value={String(hardware?.unit_count ?? 0)}
				detail={hardware
					? `${hardware.total_zones} zones · fw ${hardware.firmware_version || 'unknown'}`
					: ''}
			/>
		</section>

		<!-- Live charts -->
		<section class="grid gap-4 xl:grid-cols-2">
			<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
				{#if diagnostics.hasHistory}
					<TimeSeriesChart
						title="CPU &amp; memory utilisation"
						series={utilizationSeries}
						{timestamps}
						unit="%"
						yMin={0}
						yMax={100}
					/>
				{:else}
					<p class="py-12 text-center text-sm text-gray-500 dark:text-gray-400">
						Collecting samples…
					</p>
				{/if}
			</div>

			<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
				{#if diagnostics.hasHistory && diagnostics.hasTemperature}
					<TimeSeriesChart
						title="CPU temperature"
						series={temperatureSeries}
						{timestamps}
						unit="°C"
						yMin={0}
					/>
				{:else}
					<p class="py-12 text-center text-sm text-gray-500 dark:text-gray-400">
						{diagnostics.hasHistory
							? 'No temperature sensor reported by this system.'
							: 'Collecting samples…'}
					</p>
				{/if}
			</div>
		</section>

		<!-- Storage & memory -->
		<section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
			<h2 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">Storage &amp; memory</h2>
			<div class="space-y-4">
				{#each stats.disks as disk (disk.mount)}
					<UsageMeter
						label={disk.label}
						detail={disk.mount}
						usedBytes={disk.used_bytes}
						totalBytes={disk.total_bytes}
					/>
				{/each}

				<UsageMeter
					label="RAM"
					detail="{formatBytes(stats.memory.available_bytes)} available"
					usedBytes={stats.memory.used_bytes}
					totalBytes={stats.memory.total_bytes}
				/>

				{#if stats.memory.swap_total_bytes > 0}
					<UsageMeter
						label="Swap"
						usedBytes={stats.memory.swap_used_bytes}
						totalBytes={stats.memory.swap_total_bytes}
					/>
				{/if}
			</div>
		</section>

		<!-- Preamp controllers -->
		<section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
			<div class="mb-4 flex items-baseline justify-between gap-3">
				<h2 class="text-sm font-semibold text-gray-900 dark:text-white">Preamp controllers</h2>
				{#if hardware?.mock}
					<span
						class="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-700 dark:text-gray-300"
					>
						Mock hardware
					</span>
				{/if}
			</div>

			{#if !hardware || hardware.units.length === 0}
				<p class="text-sm text-gray-500 dark:text-gray-400">No preamp units detected.</p>
			{:else}
				<div class="grid gap-3 lg:grid-cols-2">
					{#each hardware.units as unit (unit.index)}
						<article class="rounded-lg border border-gray-200 p-3 dark:border-gray-700">
							<header class="flex items-center justify-between gap-2">
								<h3 class="font-medium text-gray-900 dark:text-white">
									{unit.index === 0 ? 'Main unit' : `Expander ${unit.index}`}
									<span class="ml-1 text-xs font-normal text-gray-500 dark:text-gray-400">
										{unit.i2c_address}
									</span>
								</h3>
								<!-- Status carries a word, not just a colour -->
								<span
									class="rounded px-2 py-0.5 text-xs font-medium {unit.online
										? 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300'
										: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300'}"
								>
									{unit.online ? 'Online' : 'Offline'}
								</span>
							</header>

							<dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
								<div class="flex justify-between gap-2">
									<dt class="text-gray-500 dark:text-gray-400">Firmware</dt>
									<dd class="font-medium tabular-nums text-gray-900 dark:text-white">
										{unit.firmware_version || '—'}
									</dd>
								</div>
								<div class="flex justify-between gap-2">
									<dt class="text-gray-500 dark:text-gray-400">Type</dt>
									<dd class="font-medium text-gray-900 dark:text-white">{unit.type}</dd>
								</div>
								<div class="flex justify-between gap-2">
									<dt class="text-gray-500 dark:text-gray-400">Board</dt>
									<dd class="font-medium text-gray-900 dark:text-white">
										{unit.board_revision || '—'}
									</dd>
								</div>
								<div class="flex justify-between gap-2">
									<dt class="text-gray-500 dark:text-gray-400">Zones</dt>
									<dd class="font-medium tabular-nums text-gray-900 dark:text-white">
										{unit.zone_base + 1}–{unit.zone_base + unit.zone_count}
									</dd>
								</div>
								{#if unit.serial > 0}
									<div class="flex justify-between gap-2">
										<dt class="text-gray-500 dark:text-gray-400">Serial</dt>
										<dd class="font-medium tabular-nums text-gray-900 dark:text-white">
											{unit.serial}
										</dd>
									</div>
								{/if}
								{#if unit.fans}
									<div class="flex justify-between gap-2">
										<dt class="text-gray-500 dark:text-gray-400">Fans</dt>
										<dd class="font-medium text-gray-900 dark:text-white">
											{unit.fans.control}{unit.fans.on ? ' · on' : ''}{unit.fans.failed
												? ' · FAILED'
												: ''}
										</dd>
									</div>
								{/if}
							</dl>

							{#if unit.temps}
								<div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 border-t border-gray-100 pt-2 text-xs dark:border-gray-700">
									<span class="text-gray-500 dark:text-gray-400">
										Amp 1–3 <span class="font-medium tabular-nums text-gray-900 dark:text-white"
											>{formatTemp(unit.temps.amp1_c)}</span
										>
									</span>
									<span class="text-gray-500 dark:text-gray-400">
										Amp 4–6 <span class="font-medium tabular-nums text-gray-900 dark:text-white"
											>{formatTemp(unit.temps.amp2_c)}</span
										>
									</span>
									<span class="text-gray-500 dark:text-gray-400">
										PSU <span class="font-medium tabular-nums text-gray-900 dark:text-white"
											>{formatTemp(unit.temps.psu1_c)}</span
										>
									</span>
								</div>
							{/if}

							{#if unit.error}
								<p class="mt-2 text-xs text-red-600 dark:text-red-400">{unit.error}</p>
							{/if}
						</article>
					{/each}
				</div>
			{/if}
		</section>

		<!-- System & daemon details -->
		<section class="grid gap-4 lg:grid-cols-2">
			<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">System</h2>
				<dl class="space-y-1.5 text-sm">
					{#each [['Hostname', stats.host.hostname], ['Model', stats.host.model || '—'], ['Operating system', stats.host.os || '—'], ['Kernel', stats.host.kernel || '—'], ['Architecture', stats.host.architecture], ['Board serial', stats.host.serial || '—'], ['Processor', stats.cpu.model], ['Load average', `${stats.cpu.load_avg_1.toFixed(2)} · ${stats.cpu.load_avg_5.toFixed(2)} · ${stats.cpu.load_avg_15.toFixed(2)}`], ['Processes', stats.cpu.total_tasks > 0 ? `${stats.cpu.processes} running of ${stats.cpu.total_tasks}` : '—']] as [term, value] (term)}
						<div class="flex justify-between gap-4">
							<dt class="shrink-0 text-gray-500 dark:text-gray-400">{term}</dt>
							<dd class="truncate text-right font-medium text-gray-900 dark:text-white">{value}</dd>
						</div>
					{/each}
				</dl>
			</div>

			<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">AmpliPi daemon</h2>
				<dl class="space-y-1.5 text-sm">
					{#each [['Version', diagnostics.version || '—'], ['Go runtime', stats.process.go_version], ['Process ID', String(stats.process.pid)], ['Process uptime', formatDuration(stats.process.uptime_seconds)], ['Goroutines', String(stats.process.goroutines)], ['Heap in use', formatBytes(stats.process.heap_alloc_bytes)], ['Heap reserved', formatBytes(stats.process.heap_sys_bytes)], ['GC cycles', String(stats.process.gc_count)], ['Network status', amplipi.info?.offline ? 'Offline' : 'Online']] as [term, value] (term)}
						<div class="flex justify-between gap-4">
							<dt class="shrink-0 text-gray-500 dark:text-gray-400">{term}</dt>
							<dd class="truncate text-right font-medium tabular-nums text-gray-900 dark:text-white">
								{value}
							</dd>
						</div>
					{/each}
				</dl>
			</div>
		</section>

		<!-- Network -->
		<section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
			<h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">Network</h2>
			{#if stats.network.length === 0}
				<p class="text-sm text-gray-500 dark:text-gray-400">No active interfaces.</p>
			{:else}
				<div class="overflow-x-auto">
					<table class="w-full text-left text-sm">
						<thead class="text-xs text-gray-500 dark:text-gray-400">
							<tr>
								<th scope="col" class="py-1 pr-4 font-medium">Interface</th>
								<th scope="col" class="py-1 pr-4 font-medium">Addresses</th>
								<th scope="col" class="py-1 pr-4 font-medium">MAC</th>
								<th scope="col" class="py-1 pr-4 font-medium">Received</th>
								<th scope="col" class="py-1 font-medium">Sent</th>
							</tr>
						</thead>
						<tbody class="text-gray-900 dark:text-white">
							{#each stats.network as iface (iface.name)}
								<tr class="border-t border-gray-100 dark:border-gray-700">
									<td class="py-1.5 pr-4 font-medium">
										{iface.name}
										<span class="ml-1 text-xs font-normal text-gray-500 dark:text-gray-400">
											{iface.up ? 'up' : 'down'}
										</span>
									</td>
									<td class="py-1.5 pr-4 tabular-nums">{iface.addresses.join(', ') || '—'}</td>
									<td class="py-1.5 pr-4 tabular-nums text-gray-500 dark:text-gray-400">
										{iface.mac || '—'}
									</td>
									<td class="py-1.5 pr-4 tabular-nums">{formatBytes(iface.rx_bytes)}</td>
									<td class="py-1.5 tabular-nums">{formatBytes(iface.tx_bytes)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>

		<!-- Stream capabilities -->
		{#if hardware && hardware.stream_support.length > 0}
			<section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
					Stream support
					<span class="font-normal text-gray-500 dark:text-gray-400">
						({availableStreams.length} of {hardware.stream_support.length} available)
					</span>
				</h2>
				<div class="flex flex-wrap gap-2">
					{#each availableStreams as cap (cap.type)}
						<span
							class="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800 dark:bg-green-900/40 dark:text-green-300"
							title={cap.binary}
						>
							✓ {cap.type}
						</span>
					{/each}
					{#each missingStreams as cap (cap.type)}
						<span
							class="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-500 dark:bg-gray-700 dark:text-gray-400"
							title={cap.reason}
						>
							✗ {cap.type}
						</span>
					{/each}
				</div>
			</section>
		{/if}
	{/if}
</div>
