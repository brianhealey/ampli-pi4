<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api';
	import { formatLogTime } from '$lib/format';
	import type { LogEntry, LogLevel, LogResult, LogSource } from '$lib/diagnostics';

	const LEVELS: { value: LogLevel; label: string }[] = [
		{ value: 'debug', label: 'Debug and above' },
		{ value: 'info', label: 'Info and above' },
		{ value: 'warn', label: 'Warnings and errors' },
		{ value: 'error', label: 'Errors only' }
	];

	const WINDOWS = [
		{ value: '', label: 'All retained' },
		{ value: '15m', label: 'Last 15 minutes' },
		{ value: '1h', label: 'Last hour' },
		{ value: '6h', label: 'Last 6 hours' },
		{ value: '1d', label: 'Last day' },
		{ value: '7d', label: 'Last 7 days' }
	];

	const LINE_COUNTS = [100, 200, 500, 1000, 2000];

	const REFRESH_INTERVAL = 3000;

	let sources = $state<LogSource[]>([]);
	let result = $state<LogResult | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Filters
	let source = $state('app');
	let level = $state<LogLevel>('debug');
	let since = $state('');
	let filter = $state('');
	let lines = $state(500);

	let autoRefresh = $state(true);
	let follow = $state(true);
	let logPane = $state<HTMLDivElement | null>(null);

	let refreshTimer: number | null = null;
	let abortController: AbortController | null = null;

	const entries = $derived(result?.entries ?? []);

	onMount(() => {
		loadSources();
		return () => teardown();
	});

	// Refetch whenever a filter changes, and (re)arm the refresh timer to match
	// the auto-refresh setting.
	$effect(() => {
		// Referenced so the effect re-runs when any filter changes.
		void source;
		void level;
		void since;
		void filter;
		void lines;
		void autoRefresh;

		// Arm the timer before fetching: fetchLogs is async and aborts the
		// previous request itself, so tearing down here would cancel the
		// request this run just issued.
		stopTimer();
		if (autoRefresh) {
			refreshTimer = window.setInterval(fetchLogs, REFRESH_INTERVAL);
		}
		fetchLogs();

		return () => stopTimer();
	});

	/** Clears the refresh timer, leaving any in-flight request alone. */
	function stopTimer() {
		if (refreshTimer !== null) {
			clearInterval(refreshTimer);
			refreshTimer = null;
		}
	}

	/** Stops polling entirely — used when the page unmounts. */
	function teardown() {
		stopTimer();
		abortController?.abort();
		abortController = null;
	}

	async function loadSources() {
		try {
			const response = await api.getLogSources();
			sources = response.sources;
		} catch (err) {
			console.error('Failed to load log sources:', err);
		}
	}

	async function fetchLogs() {
		abortController?.abort();
		abortController = new AbortController();
		const signal = abortController.signal;

		try {
			const next = await api.getLogs(
				{ source, level, lines, since: since || undefined, filter: filter || undefined },
				signal
			);
			if (signal.aborted) return;

			result = next;
			error = null;

			if (follow) {
				// Wait for the new rows to render before scrolling to the end.
				await tick();
				logPane?.scrollTo({ top: logPane.scrollHeight });
			}
		} catch (err) {
			if (err instanceof Error && err.name === 'AbortError') return;
			error = err instanceof Error ? err.message : 'Failed to load logs';
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	/** Level styling pairs colour with the level word, never colour alone. */
	function levelClass(entryLevel: LogLevel): string {
		switch (entryLevel) {
			case 'error':
				return 'text-red-600 dark:text-red-400';
			case 'warn':
				return 'text-amber-600 dark:text-amber-400';
			case 'debug':
				return 'text-gray-400 dark:text-gray-500';
			default:
				return 'text-blue-600 dark:text-blue-400';
		}
	}

	function downloadLogs() {
		const text = entries
			.map((e: LogEntry) => `${e.time} ${e.level.toUpperCase()} ${e.unit ? `[${e.unit}] ` : ''}${e.message}`)
			.join('\n');
		const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
		const link = document.createElement('a');
		link.href = url;
		link.download = `amplipi-${source}-logs.txt`;
		link.click();
		URL.revokeObjectURL(url);
	}
</script>

<svelte:head>
	<title>Logs — AmpliPi</title>
</svelte:head>

<div class="flex h-full flex-col gap-4 p-4 md:p-6">
	<header class="flex flex-wrap items-baseline justify-between gap-3">
		<div>
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Debug logs</h1>
			<p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
				{sources.find((s) => s.id === source)?.description ?? 'Device and application logs'}
			</p>
		</div>
		<button
			type="button"
			onclick={downloadLogs}
			disabled={entries.length === 0}
			class="rounded-lg border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100 disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
		>
			Download
		</button>
	</header>

	<!-- Filters, in one row above the log pane -->
	<div class="flex flex-wrap items-end gap-3">
		<label class="flex flex-col gap-1">
			<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Source</span>
			<select
				bind:value={source}
				class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
			>
				{#each sources as s (s.id)}
					<option value={s.id}>{s.label}</option>
				{/each}
			</select>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Level</span>
			<select
				bind:value={level}
				class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
			>
				{#each LEVELS as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			</select>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Time range</span>
			<select
				bind:value={since}
				class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
			>
				{#each WINDOWS as option (option.value)}
					<option value={option.value}>{option.label}</option>
				{/each}
			</select>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Lines</span>
			<select
				bind:value={lines}
				class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
			>
				{#each LINE_COUNTS as option (option)}
					<option value={option}>{option}</option>
				{/each}
			</select>
		</label>

		<label class="flex min-w-48 flex-1 flex-col gap-1">
			<span class="text-xs font-medium text-gray-500 dark:text-gray-400">Search</span>
			<input
				type="search"
				bind:value={filter}
				placeholder="Filter messages…"
				class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
			/>
		</label>

		<label class="flex items-center gap-2 py-1.5 text-sm text-gray-700 dark:text-gray-300">
			<input type="checkbox" bind:checked={autoRefresh} class="rounded" />
			Auto-refresh
		</label>

		<label class="flex items-center gap-2 py-1.5 text-sm text-gray-700 dark:text-gray-300">
			<input type="checkbox" bind:checked={follow} class="rounded" />
			Follow
		</label>
	</div>

	{#if error}
		<div class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
			{error}
		</div>
	{/if}

	{#if result && !result.available}
		<div
			class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
		>
			This log source is not readable on this device: {result.reason}
		</div>
	{/if}

	<!-- Log pane -->
	<div
		bind:this={logPane}
		class="min-h-64 flex-1 overflow-auto rounded-lg border border-gray-200 bg-white p-3 font-mono text-xs leading-relaxed dark:border-gray-700 dark:bg-gray-900"
	>
		{#if loading && entries.length === 0}
			<p class="text-gray-500 dark:text-gray-400">Loading logs…</p>
		{:else if entries.length === 0}
			<p class="text-gray-500 dark:text-gray-400">No log entries match the current filters.</p>
		{:else}
			{#each entries as entry, i (`${entry.time}-${i}`)}
				<div class="flex gap-3 whitespace-pre-wrap break-words py-0.5">
					<span class="shrink-0 tabular-nums text-gray-400 dark:text-gray-500">
						{formatLogTime(entry.time)}
					</span>
					<span class="w-12 shrink-0 font-semibold uppercase {levelClass(entry.level)}">
						{entry.level}
					</span>
					{#if entry.unit}
						<span class="shrink-0 text-gray-500 dark:text-gray-400">{entry.unit}</span>
					{/if}
					<span class="min-w-0 text-gray-800 dark:text-gray-200">{entry.message}</span>
				</div>
			{/each}
		{/if}
	</div>

	<p class="text-xs text-gray-500 dark:text-gray-400">
		Showing {entries.length} entries{autoRefresh ? ' · refreshing every 3s' : ''}
	</p>
</div>
