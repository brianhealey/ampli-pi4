<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Source, Zone, Group } from '$lib/types';
	import { filterZonesByGroup, getSourceGroups, getGroupZones } from '$lib/grouping';

	let expandedGroups = $state<Set<number>>(new Set());
	// Track previous slider position (0-100) for each group to calculate deltas
	let groupSliderPos = $state<Map<number, number>>(new Map());

	async function updateZone(zoneId: number, update: { mute?: boolean; vol?: number }) {
		try {
			await api.updateZone(zoneId, update);
		} catch (err) {
			console.error('Failed to update zone:', err);
		}
	}

	async function updateGroup(groupId: number, update: { mute?: boolean; vol_delta?: number }) {
		const start = performance.now();
		console.log(`[API] updateGroup(${groupId}, ${JSON.stringify(update)}) - START`);
		try {
			await api.updateGroup(groupId, update);
			const elapsed = performance.now() - start;
			console.log(`[API] updateGroup(${groupId}) - DONE in ${elapsed.toFixed(1)}ms`);
		} catch (err) {
			const elapsed = performance.now() - start;
			console.error(`[API] updateGroup(${groupId}) - FAILED after ${elapsed.toFixed(1)}ms:`, err);
		}
	}

	function volFToPercent(vol_f: number): number {
		return Math.round(vol_f * 100);
	}

	function percentToVolF(percent: number): number {
		return percent / 100;
	}

	async function assignStreamToSource(sourceId: number, streamId: number) {
		try {
			const input = streamId >= 0 ? `stream=${streamId}` : '';
			await api.updateSource(sourceId, { input });
		} catch (err) {
			console.error('Failed to assign stream:', err);
		}
	}

	function toggleGroupExpanded(groupId: number) {
		if (expandedGroups.has(groupId)) {
			expandedGroups.delete(groupId);
		} else {
			expandedGroups.add(groupId);
		}
		expandedGroups = expandedGroups;
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

	function getSourceZonesAndGroups(source: Source) {
		const zones = getSourceZones(source);
		const groups = getSourceGroups(source.id, amplipi.zones, amplipi.groups);
		const { grouped, standalone } = filterZonesByGroup(zones, groups);
		return { groups, grouped, standalone };
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
			{@const { groups, grouped, standalone } = getSourceZonesAndGroups(source)}
			{@const stream = getSourceStream(source)}
			{@const totalZoneCount = standalone.length + Array.from(grouped.values()).reduce((sum, zones) => sum + zones.length, 0)}

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
							{totalZoneCount} zone{totalZoneCount !== 1 ? 's' : ''}
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

				<!-- Groups and zones list -->
				{#if groups.length > 0 || standalone.length > 0}
					<div class="space-y-3">
						<!-- Groups first -->
						{#each groups as group (group.id)}
							{@const groupZones = grouped.get(group.id) || []}
							{@const isExpanded = expandedGroups.has(group.id)}
							{#if groupZones.length > 0}
								<div class="rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
									<!-- Group header -->
									<div class="mb-2 flex items-center justify-between">
										<button
											onclick={() => toggleGroupExpanded(group.id)}
											class="flex items-center gap-2 text-left"
										>
											<span class="text-sm font-medium text-purple-900 dark:text-purple-300">
												{isExpanded ? '▼' : '▶'} {group.name}
											</span>
											<span class="text-xs text-purple-700 dark:text-purple-400">
												({groupZones.length} zone{groupZones.length !== 1 ? 's' : ''})
											</span>
										</button>
										<button
											onclick={() => updateGroup(group.id, { mute: !group.mute })}
											class="rounded p-1 hover:bg-purple-100 dark:hover:bg-purple-800"
											class:text-gray-400={group.mute}
											class:text-purple-600={!group.mute}
											class:dark:text-purple-400={!group.mute}
										>
											{group.mute ? '🔇' : '🔊'}
										</button>
									</div>

									<!-- Group volume slider -->
									<div class="mb-3 flex items-center gap-2">
										<span class="text-xs text-purple-700 dark:text-purple-400">
											{volFToPercent(group.vol_f ?? 0)}%
										</span>
										<input
											type="range"
											min="0"
											max="100"
											value={volFToPercent(group.vol_f ?? 0)}
											oninput={(e) => {
												const newPos = parseInt(e.currentTarget.value);
												const oldPos = groupSliderPos.get(group.id) ?? volFToPercent(group.vol_f ?? 0);
												const deltaPercent = newPos - oldPos;

												// Convert percent delta to dB delta (79 dB range)
												const volDelta = Math.round((deltaPercent / 100) * 79);

												console.log(`[Group ${group.id}] Slider: ${oldPos}% -> ${newPos}% (delta: ${deltaPercent}%, ${volDelta}dB)`);

												// Track new position for next delta calculation
												groupSliderPos.set(group.id, newPos);

												if (volDelta !== 0) {
													console.log(`[Group ${group.id}] Sending vol_delta: ${volDelta}`);
													updateGroup(group.id, { vol_delta: volDelta, mute: false });
												} else {
													console.log(`[Group ${group.id}] No change (volDelta === 0)`);
												}
											}}
											class="flex-1 accent-purple-600"
										/>
									</div>

									<!-- Expanded group zones -->
									{#if isExpanded}
										<div class="space-y-2 border-t border-purple-200 p-3 pt-2 dark:border-purple-800">
											{#each groupZones as zone (zone.id)}
												<div class="rounded bg-white p-2 dark:bg-gray-800">
													<div class="mb-1 flex items-center justify-between">
														<span class="text-xs font-medium text-gray-700 dark:text-gray-300">
															{zone.name}
														</span>
														<button
															onclick={() => updateZone(zone.id, { mute: !zone.mute })}
															class="rounded p-0.5 text-xs hover:bg-gray-100 dark:hover:bg-gray-700"
															class:text-gray-400={zone.mute}
															class:text-blue-600={!zone.mute}
															class:dark:text-blue-400={!zone.mute}
														>
															{zone.mute ? '🔇' : '🔊'}
														</button>
													</div>
													<div class="flex items-center gap-1">
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
									{/if}
								</div>
							{/if}
						{/each}

						<!-- Standalone zones (not in any group) -->
						{#each standalone as zone (zone.id)}
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
