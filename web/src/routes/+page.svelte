<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Source, Zone } from '$lib/types';

	async function updateZone(zoneId: number, update: { mute?: boolean; vol?: number }) {
		try {
			await api.updateZone(zoneId, update);
		} catch (err) {
			console.error('Failed to update zone:', err);
		}
	}

	async function assignStreamToSource(sourceId: number, streamId: number) {
		try {
			const input = streamId >= 0 ? `stream=${streamId}` : '';
			await api.updateSource(sourceId, { input });
		} catch (err) {
			console.error('Failed to assign stream:', err);
		}
	}

	function getSourceStream(source: Source) {
		if (!source.input.startsWith('stream=')) return null;
		const streamId = parseInt(source.input.replace('stream=', ''));
		return amplipi.getStream(streamId);
	}

	function getSourceStreamId(source: Source): number {
		if (!source.input.startsWith('stream=')) return -1;
		return parseInt(source.input.replace('stream=', ''));
	}

	function getSourceZones(source: Source) {
		return amplipi.zones.filter((z) => z.source_id === source.id && !z.disabled);
	}

	function dbToPercent(db: number, min: number = -79, max: number = 0): number {
		return Math.round(((db - min) / (max - min)) * 100);
	}

	function percentToDb(percent: number, min: number = -79, max: number = 0): number {
		return Math.round(min + (percent / 100) * (max - min));
	}
</script>

<div class="p-4 md:p-6">
	<div class="mb-6">
		<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Sources</h2>
		<p class="text-sm text-gray-600 dark:text-gray-400">Control your audio sources and zones</p>
	</div>

	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
		{#each amplipi.sources as source (source.id)}
			{@const zones = getSourceZones(source)}
			{@const stream = getSourceStream(source)}

			<div
				class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800"
			>
				<!-- Source header -->
				<div class="mb-4">
					<div class="mb-2 flex items-start justify-between">
						<div>
							<h3 class="font-semibold text-gray-900 dark:text-white">{source.name}</h3>
							{#if stream}
								<p class="text-sm text-gray-600 dark:text-gray-400">{stream.name}</p>
								{#if stream.info?.state}
									<span
										class={`mt-1 inline-block rounded px-2 py-0.5 text-xs font-medium ${stream.info.state === 'playing' ? 'bg-green-100 text-green-700 dark:bg-green-900/20 dark:text-green-400' : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'}`}
									>
										{stream.info.state}
									</span>
								{/if}
							{:else}
								<p class="text-sm text-gray-500 dark:text-gray-500">No stream</p>
							{/if}
						</div>
						<span class="rounded-full bg-blue-100 px-2 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/20 dark:text-blue-400">
							{zones.length} zone{zones.length !== 1 ? 's' : ''}
						</span>
					</div>

					<!-- Stream selector -->
					<div>
						<select
							value={getSourceStreamId(source)}
							onchange={(e) => assignStreamToSource(source.id, parseInt(e.currentTarget.value))}
							class="w-full rounded border border-gray-300 bg-white px-2 py-1 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
						>
							<option value={-1}>No stream</option>
							{#each amplipi.streams as s (s.id)}
								<option value={s.id}>{s.name} ({s.type})</option>
							{/each}
						</select>
					</div>
				</div>

				<!-- Zones list -->
				{#if zones.length > 0}
					<div class="space-y-3">
						{#each zones as zone (zone.id)}
							<div class="rounded-lg bg-gray-50 p-3 dark:bg-gray-700/50">
								<div class="mb-2 flex items-center justify-between">
									<span class="text-sm font-medium text-gray-900 dark:text-white">{zone.name}</span>
									<button
										onclick={() => updateZone(zone.id, { mute: !zone.mute })}
										class="rounded p-1 hover:bg-gray-200 dark:hover:bg-gray-600"
										class:text-gray-400={zone.mute}
										class:text-blue-600={!zone.mute}
										class:dark:text-blue-400={!zone.mute}
									>
										{zone.mute ? '🔇' : '🔊'}
									</button>
								</div>

								<!-- Volume slider -->
								<div class="flex items-center gap-2">
									<span class="text-xs text-gray-500 dark:text-gray-400">
										{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}%
									</span>
									<input
										type="range"
										min="0"
										max="100"
										value={dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}
										oninput={(e) => {
											const percent = parseInt(e.currentTarget.value);
											const db = percentToDb(percent, zone.vol_min, zone.vol_max);
											updateZone(zone.id, { vol: db });
										}}
										class="flex-1 accent-blue-600"
									/>
								</div>
							</div>
						{/each}
					</div>
				{:else}
					<div class="rounded-lg bg-gray-50 p-4 text-center dark:bg-gray-700/50">
						<p class="text-sm text-gray-500 dark:text-gray-400">No zones connected</p>
					</div>
				{/if}
			</div>
		{/each}
	</div>

	{#if amplipi.sources.length === 0}
		<div class="rounded-lg border-2 border-dashed border-gray-300 p-12 text-center dark:border-gray-700">
			<p class="text-gray-500 dark:text-gray-400">No sources available</p>
		</div>
	{/if}
</div>
