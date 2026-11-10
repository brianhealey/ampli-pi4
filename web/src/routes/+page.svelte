<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api, apiErrorMessage } from '$lib/api';
	import type { Zone, Group, Source, Stream } from '$lib/types';
	import { dbToPercent, percentToDb, volFToPercent } from '$lib/volume';

	// Which source name is being edited, and the in-flight text.
	let editingSource = $state<number | null>(null);
	let editingName = $state('');

	// Master slider position per source. The hardware has no per-source volume —
	// it is applied as a relative shift across that source's zones — so the
	// slider needs its own position to measure movement against.
	let masterPos = $state<Map<number, number>>(new Map());

	const streams = $derived(amplipi.streams.filter((s) => !s.disabled));

	// RCA input n is wired to source n's analog input; there is no crossbar, so
	// an RCA stream can only ever sit on that one source.
	function pinnedSource(stream: { type: string; config?: Record<string, unknown> }): number | null {
		if (stream.type === 'rca' && stream.config?.index !== undefined) {
			return stream.config.index as number;
		}
		return null;
	}

	function streamsFor(sourceId: number) {
		return streams.filter((s) => {
			const pinned = pinnedSource(s);
			return pinned === null || pinned === sourceId;
		});
	}

	// RCA and Aux are analog passthroughs; every other stream type is digital and
	// needs a DAC feeding the source. Units without the USB DAC only have one on
	// the first source.
	function isDigital(stream: Stream): boolean {
		return stream.type !== 'rca' && stream.type !== 'aux';
	}

	function digitalBlocked(source: Source, stream: Stream): boolean {
		return source.has_dac === false && isDigital(stream);
	}

	const dacSourceNames = $derived(
		amplipi.sources
			.filter((s) => s.has_dac !== false)
			.map((s) => s.name)
			.join(', ')
	);

	// Last failed input change per source, shown under its selector.
	let inputErrors = $state<Record<number, string>>({});

	function groupsOn(sourceId: number): Group[] {
		return amplipi.groups.filter((g) =>
			g.zones.some((id) => {
				const z = amplipi.getZone(id);
				return z !== undefined && z.source_id === sourceId && !z.disabled;
			})
		);
	}

	// Zones on this source that are not already shown as part of a group.
	function zonesOn(sourceId: number): Zone[] {
		const grouped = new Set(groupsOn(sourceId).flatMap((g) => g.zones));
		return amplipi.zones.filter(
			(z) => z.source_id === sourceId && !z.disabled && !grouped.has(z.id)
		);
	}

	function allZonesOn(sourceId: number): Zone[] {
		return amplipi.zones.filter((z) => z.source_id === sourceId && !z.disabled);
	}

	function nowPlaying(src: Source): string {
		if (!src.input) return 'No input';
		if (src.input.startsWith('stream=')) {
			return amplipi.getStream(parseInt(src.input.substring(7)))?.name ?? src.input;
		}
		return src.input;
	}

	function masterValue(sourceId: number): number {
		const cached = masterPos.get(sourceId);
		if (cached !== undefined) return cached;
		const zones = allZonesOn(sourceId);
		if (zones.length === 0) return 0;
		const avg =
			zones.reduce((sum, z) => sum + dbToPercent(z.vol, z.vol_min, z.vol_max), 0) / zones.length;
		return Math.round(avg);
	}

	async function setSourceInput(source: Source, input: string, select: HTMLSelectElement) {
		try {
			await api.updateSource(source.id, { input });
			delete inputErrors[source.id];
		} catch (err) {
			console.error('Failed to set source input:', err);
			inputErrors[source.id] = apiErrorMessage(err);
			// The change was refused: show the input that is actually playing.
			select.value = source.input || '';
		}
	}

	async function commitSourceName(sourceId: number) {
		const name = editingName.trim();
		editingSource = null;
		if (!name) return;
		try {
			await api.updateSource(sourceId, { name });
		} catch (err) {
			console.error('Failed to rename source:', err);
		}
	}

	async function setZoneVolume(zone: Zone, percent: number) {
		try {
			await api.updateZone(zone.id, { vol: percentToDb(percent, zone.vol_min, zone.vol_max) });
		} catch (err) {
			console.error('Failed to set zone volume:', err);
		}
	}

	async function toggleZoneMute(zone: Zone) {
		try {
			await api.updateZone(zone.id, { mute: !zone.mute });
		} catch (err) {
			console.error('Failed to mute zone:', err);
		}
	}

	async function setGroupVolume(group: Group, percent: number) {
		const old = volFToPercent(group.vol_f ?? 0);
		const delta = Math.round(((percent - old) / 100) * 79);
		if (delta === 0) return;
		try {
			await api.updateGroup(group.id, { vol_delta: delta });
		} catch (err) {
			console.error('Failed to set group volume:', err);
		}
	}

	async function toggleGroupMute(group: Group) {
		try {
			await api.updateGroup(group.id, { mute: !group.mute });
		} catch (err) {
			console.error('Failed to mute group:', err);
		}
	}

	// Shift every zone on this source by the same amount, preserving the balance
	// between rooms rather than flattening them all to one level.
	async function setMasterVolume(sourceId: number, percent: number) {
		const previous = masterValue(sourceId);
		masterPos.set(sourceId, percent);
		masterPos = new Map(masterPos);

		const deltaF = (percent - previous) / 100;
		if (deltaF === 0) return;

		const ids = allZonesOn(sourceId).map((z) => z.id);
		if (ids.length === 0) return;
		try {
			await api.updateZones(ids, { vol_delta_f: deltaF });
		} catch (err) {
			console.error('Failed to set master volume:', err);
		}
	}
</script>

<div class="p-4 md:p-6">
	<div class="mb-6">
		<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Home</h2>
		<p class="text-sm text-gray-600 dark:text-gray-400">
			Each source carries one input and feeds the zones assigned to it.
		</p>
	</div>

	<div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
		{#each amplipi.sources as source (source.id)}
			{@const groups = groupsOn(source.id)}
			{@const zones = zonesOn(source.id)}
			{@const total = allZonesOn(source.id).length}

			<div
				class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
			>
				<!-- Source name (click to rename) -->
				<div class="flex items-start justify-between gap-2">
					{#if editingSource === source.id}
						<!-- svelte-ignore a11y_autofocus -->
						<input
							type="text"
							bind:value={editingName}
							autofocus
							onblur={() => commitSourceName(source.id)}
							onkeydown={(e) => {
								if (e.key === 'Enter') commitSourceName(source.id);
								if (e.key === 'Escape') editingSource = null;
							}}
							class="w-full rounded border border-blue-500 bg-white px-2 py-1 text-lg font-semibold dark:bg-gray-700 dark:text-white"
							aria-label="Source name"
						/>
					{:else}
						<button
							onclick={() => {
								editingSource = source.id;
								editingName = source.name;
							}}
							class="text-left text-lg font-semibold text-gray-900 hover:underline dark:text-white"
							title="Rename source"
						>
							{source.name}
						</button>
					{/if}
					<span class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
						{total} zone{total === 1 ? '' : 's'}
					</span>
				</div>

				<p class="mt-0.5 text-sm text-gray-600 dark:text-gray-400">{nowPlaying(source)}</p>

				<!-- Input -->
				<div class="mt-3">
					<label
						for="source-input-{source.id}"
						class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">Input</label
					>
					<select
						id="source-input-{source.id}"
						value={source.input || ''}
						onchange={(e) => setSourceInput(source, e.currentTarget.value, e.currentTarget)}
						class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
					>
						<option value="">None</option>
						{#each streamsFor(source.id) as stream (stream.id)}
							<option value="stream={stream.id}" disabled={digitalBlocked(source, stream)}>
								{stream.name}{stream.type === 'rca' && stream.config?.index !== undefined
									? ` (RCA ${(stream.config.index as number) + 1})`
									: ''}{digitalBlocked(source, stream) ? ' (needs a DAC)' : ''}
							</option>
						{/each}
					</select>
					{#if source.has_dac === false}
						<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
							No DAC on this source: only its RCA input plays here. Digital streams can play
							on {dacSourceNames || 'no source on this unit'}.
						</p>
					{/if}
					{#if inputErrors[source.id]}
						<p class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
							{inputErrors[source.id]}
						</p>
					{/if}
				</div>

				<!-- Master volume for everything on this source -->
				{#if total > 0}
					<div class="mt-4 rounded-lg bg-gray-50 p-3 dark:bg-gray-900/40">
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-gray-700 dark:text-gray-300">All zones</span>
							<span class="text-xs text-gray-600 dark:text-gray-400"
								>{masterValue(source.id)}%</span
							>
						</div>
						<input
							type="range"
							min="0"
							max="100"
							value={masterValue(source.id)}
							onchange={(e) => setMasterVolume(source.id, parseInt(e.currentTarget.value))}
							class="mt-1 w-full"
							aria-label="Master volume for {source.name}"
						/>
						<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
							Shifts every zone together, keeping their relative levels.
						</p>
					</div>
				{/if}

				<!-- Groups on this source -->
				{#each groups as group (group.id)}
					<div class="mt-3 rounded-lg border border-purple-200 p-3 dark:border-purple-800">
						<div class="flex items-center justify-between">
							<span class="text-sm font-semibold text-purple-800 dark:text-purple-300">
								{group.name}
								<span class="font-normal text-purple-600 dark:text-purple-400">
									({group.zones.length} zones)
								</span>
							</span>
							<button
								onclick={() => toggleGroupMute(group)}
								class="rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-700"
								aria-label={group.mute ? `Unmute ${group.name}` : `Mute ${group.name}`}
								aria-pressed={!group.mute}
							>
								<span class="text-lg" aria-hidden="true">{group.mute ? '🔇' : '🔊'}</span>
							</button>
						</div>
						<input
							type="range"
							min="0"
							max="100"
							value={volFToPercent(group.vol_f ?? 0)}
							onchange={(e) => setGroupVolume(group, parseInt(e.currentTarget.value))}
							class="mt-1 w-full"
							aria-label="Volume for {group.name}"
						/>
					</div>
				{/each}

				<!-- Ungrouped zones on this source -->
				{#each zones as zone (zone.id)}
					<div class="mt-3 rounded-lg border border-gray-200 p-3 dark:border-gray-700">
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-gray-900 dark:text-white">{zone.name}</span>
							<div class="flex items-center gap-2">
								<span class="text-xs text-gray-600 dark:text-gray-400">
									{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}%
								</span>
								<button
									onclick={() => toggleZoneMute(zone)}
									class="rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-700"
									aria-label={zone.mute ? `Unmute ${zone.name}` : `Mute ${zone.name}`}
									aria-pressed={!zone.mute}
								>
									<span class="text-lg" aria-hidden="true">{zone.mute ? '🔇' : '🔊'}</span>
								</button>
							</div>
						</div>
						<input
							type="range"
							min="0"
							max="100"
							value={dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}
							onchange={(e) => setZoneVolume(zone, parseInt(e.currentTarget.value))}
							class="mt-1 w-full"
							aria-label="Volume for {zone.name}"
						/>
					</div>
				{/each}

				{#if total === 0}
					<p
						class="mt-3 rounded-lg border-2 border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400"
					>
						No zones assigned — assign some on the Zones page.
					</p>
				{/if}
			</div>
		{/each}
	</div>
</div>
