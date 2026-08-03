<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Zone } from '$lib/types';
	import { filterZonesByGroup } from '$lib/grouping';
	import { dbToPercent, percentToDb, volFToPercent } from '$lib/volume';

	let expandedGroups = $state<Set<number>>(new Set());
	// Track previous slider position (0-100) for each group to calculate deltas
	let groupSliderPos = $state<Map<number, number>>(new Map());

	// Filter zones into groups and standalone
	const enabledZones = $derived(amplipi.zones.filter((z) => !z.disabled));
	const { grouped, standalone } = $derived(filterZonesByGroup(enabledZones, amplipi.groups));

	// Every zone the hardware reports, for the availability section. Zones on
	// hardware added after the config was written arrive disabled, so this is
	// the only place they can be switched on.
	const allZones = $derived([...amplipi.zones].sort((a, b) => a.id - b.id));

	async function updateZone(zoneId: number, update: Partial<Zone>) {
		try {
			await api.updateZone(zoneId, update);
		} catch (err) {
			console.error('Failed to update zone:', err);
		}
	}

	// Disabling a zone also mutes it, so a zone can never be left driving a
	// speaker while hidden from the controls above.
	async function setZoneEnabled(zone: Zone, enabled: boolean) {
		await updateZone(zone.id, enabled ? { disabled: false } : { disabled: true, mute: true });
	}

	async function handleGroupInputChange(groupId: number, inputValue: string) {
		if (inputValue === 'none') {
			// Disconnect from source
			await api.updateGroup(groupId, { source_id: -1 });
			return;
		}

		// Parse stream=<id> format
		if (inputValue.startsWith('stream=')) {
			const streamId = parseInt(inputValue.substring(7));
			const stream = amplipi.getStream(streamId);

			if (!stream) {
				console.error('Stream not found:', streamId);
				return;
			}

			// For RCA streams, the source_id must match the stream's index
			if (stream.type === 'rca' && stream.config?.index !== undefined) {
				const sourceId = stream.config.index as number;
				// First update the source's input
				await api.updateSource(sourceId, { input: inputValue });
				// Then assign the group to that source
				await api.updateGroup(groupId, { source_id: sourceId });
			} else {
				// For non-RCA streams, assign to source 0 by default
				await api.updateSource(0, { input: inputValue });
				await api.updateGroup(groupId, { source_id: 0 });
			}
		}
	}

	function availableStreamsForGroup(group: typeof amplipi.groups[0]): typeof amplipi.streams {
		// Return all streams that are not disabled
		return amplipi.streams.filter((stream) => !stream.disabled);
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

	function toggleGroupExpanded(groupId: number) {
		// Create a new Set to ensure reactivity triggers
		const newExpanded = new Set(expandedGroups);
		if (newExpanded.has(groupId)) {
			newExpanded.delete(groupId);
		} else {
			newExpanded.add(groupId);
		}
		expandedGroups = newExpanded;
	}

	function availableStreamsForZone(_zone: Zone): typeof amplipi.streams {
		// Return all streams that are not disabled
		return amplipi.streams.filter((stream) => !stream.disabled);
	}

	async function handleSourceInputChange(zoneId: number, inputValue: string) {
		if (inputValue === 'none') {
			// Disconnect from source
			await updateZone(zoneId, { source_id: -1 });
			return;
		}

		// Parse stream=<id> format
		if (inputValue.startsWith('stream=')) {
			const streamId = parseInt(inputValue.substring(7));
			const stream = amplipi.getStream(streamId);

			if (!stream) {
				console.error('Stream not found:', streamId);
				return;
			}

			// For RCA streams, the source_id must match the stream's index
			if (stream.type === 'rca' && stream.config?.index !== undefined) {
				const sourceId = stream.config.index as number;
				// First update the source's input
				await api.updateSource(sourceId, { input: inputValue });
				// Then connect the zone to that source
				await updateZone(zoneId, { source_id: sourceId, mute: false });
			} else {
				// For non-RCA streams, we need to assign the stream to a source first
				// Find the zone's current source or use source 0
				const zone = amplipi.zones.find((z) => z.id === zoneId);
				const sourceId = zone && zone.source_id >= 0 ? zone.source_id : 0;

				// Update the source's input
				await api.updateSource(sourceId, { input: inputValue });
				// Connect zone to source
				await updateZone(zoneId, { source_id: sourceId, mute: false });
			}
		}
	}
</script>

<div class="p-4 md:p-6">
	<div class="mb-6">
		<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Zones & Groups</h2>
		<p class="text-sm text-gray-600 dark:text-gray-400">Manage audio zones and groups</p>
	</div>

	<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
		<!-- Groups first -->
		{#each amplipi.groups as group (group.id)}
			{@const groupZones = grouped.get(group.id) || []}
			{@const isExpanded = expandedGroups.has(group.id)}
			{#if groupZones.length > 0}
				<div
					class="rounded-lg border border-purple-200 bg-purple-50 p-4 shadow-sm dark:border-purple-800 dark:bg-purple-900/20"
				>
					<!-- Group header -->
					<div class="mb-4">
						<div class="mb-2 flex items-start justify-between">
							<button
								onclick={() => toggleGroupExpanded(group.id)}
								class="flex-1 text-left"
								aria-expanded={isExpanded}
								aria-controls="group-zones-{group.id}"
							>
								<h3 class="font-semibold text-purple-900 dark:text-purple-300">
									{isExpanded ? '▼' : '▶'} {group.name}
								</h3>
								<p class="text-sm text-purple-700 dark:text-purple-400">
									{groupZones.length} zone{groupZones.length !== 1 ? 's' : ''}
								</p>
							</button>

							<button
								onclick={() => updateGroup(group.id, { mute: !group.mute })}
								class="rounded-lg p-2 hover:bg-purple-100 dark:hover:bg-purple-800"
								class:text-gray-400={group.mute}
								class:text-purple-600={!group.mute}
								class:dark:text-purple-400={!group.mute}
								aria-label={group.mute ? `Unmute ${group.name}` : `Mute ${group.name}`}
								aria-pressed={!group.mute}
							>
								<span class="text-2xl" aria-hidden="true">{group.mute ? '🔇' : '🔊'}</span>
							</button>
						</div>

						<!-- Group volume control -->
						<div class="space-y-2">
							<div class="flex items-center justify-between">
								<span class="text-sm font-medium text-purple-700 dark:text-purple-300">Group Volume</span>
								<span class="text-sm text-purple-600 dark:text-purple-400">
									{volFToPercent(group.vol_f ?? 0)}%
								</span>
							</div>

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
								class="w-full accent-purple-600"
								aria-label="Group volume for {group.name}"
								aria-valuemin={0}
								aria-valuemax={100}
								aria-valuenow={volFToPercent(group.vol_f ?? 0)}
								aria-valuetext="{volFToPercent(group.vol_f ?? 0)} percent"
							/>

							<div class="flex items-center justify-between text-xs text-purple-500 dark:text-purple-400">
								<span>0%</span>
								<span>100%</span>
							</div>
						</div>
					</div>

					<!-- Group input selector -->
					<div class="mt-3">
						<label for="group-input-{group.id}" class="mb-1 block text-sm font-medium text-purple-700 dark:text-purple-300">
							Group Input
						</label>
						<select
							id="group-input-{group.id}"
							value={group.source_id !== undefined && group.source_id >= 0 ? amplipi.getSource(group.source_id)?.input || '' : 'none'}
							onchange={(e) => handleGroupInputChange(group.id, e.currentTarget.value)}
							class="w-full rounded border border-purple-300 bg-white px-2 py-1 text-sm focus:border-purple-500 focus:ring-2 focus:ring-purple-500 dark:border-purple-700 dark:bg-purple-900/30 dark:text-white"
						>
							<option value="none">No input</option>
							{#each availableStreamsForGroup(group) as stream (stream.id)}
								<option value="stream={stream.id}">
									{stream.name}
									{#if stream.type === 'rca' && stream.config?.index !== undefined}
										(RCA Input {(stream.config.index as number) + 1})
									{/if}
								</option>
							{/each}
						</select>
						{#if group.source_id !== undefined && group.source_id >= 0}
							{@const groupSource = amplipi.getSource(group.source_id)}
							{#if groupSource}
								<p class="mt-1 text-xs text-purple-600 dark:text-purple-400">
									All zones in this group play: {groupSource.name}
								</p>
							{/if}
						{/if}
					</div>

				<!-- Expanded group zones -->
					{#if isExpanded}
						<div id="group-zones-{group.id}" class="space-y-3 border-t border-purple-200 pt-4 dark:border-purple-800">
							<h4 class="text-xs font-semibold uppercase text-purple-700 dark:text-purple-400">
								Member Zones
							</h4>
							{#each groupZones as zone (zone.id)}
								<div class="rounded-lg border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800">
									<!-- Zone header -->
									<div class="mb-3 flex items-start justify-between">
										<div class="flex-1">
											<h4 class="text-sm font-semibold text-gray-900 dark:text-white">{zone.name}</h4>
											<p class="text-xs text-purple-600 dark:text-purple-400">
												Source controlled by group
											</p>
										</div>

										<button
											onclick={() => updateZone(zone.id, { mute: !zone.mute })}
											class="rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-700"
											class:text-gray-400={zone.mute}
											class:text-blue-600={!zone.mute}
											class:dark:text-blue-400={!zone.mute}
											aria-label={zone.mute ? `Unmute ${zone.name}` : `Mute ${zone.name}`}
											aria-pressed={!zone.mute}
										>
											<span class="text-lg" aria-hidden="true">{zone.mute ? '🔇' : '🔊'}</span>
										</button>
									</div>

									<!-- Volume control -->
									<div class="space-y-1">
										<div class="flex items-center justify-between">
											<span class="text-xs font-medium text-gray-700 dark:text-gray-300">Volume</span>
											<span class="text-xs text-gray-600 dark:text-gray-400">
												{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}%
											</span>
										</div>

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
											class="w-full accent-blue-600"
											aria-label="Volume for {zone.name}"
											aria-valuemin={0}
											aria-valuemax={100}
											aria-valuenow={dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}
											aria-valuetext="{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)} percent"
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
			{@const source = amplipi.getSource(zone.source_id)}

			<div
				class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800"
			>
				<!-- Zone header -->
				<div class="mb-4 flex items-start justify-between">
					<div class="flex-1">
						<h3 class="font-semibold text-gray-900 dark:text-white">{zone.name}</h3>
						{#if source}
							<p class="text-sm text-gray-600 dark:text-gray-400">{source.name}</p>
						{:else}
							<p class="text-sm text-gray-500 dark:text-gray-500">No source</p>
						{/if}
					</div>

					<button
						onclick={() => updateZone(zone.id, { mute: !zone.mute })}
						class="rounded-lg p-2 hover:bg-gray-100 dark:hover:bg-gray-700"
						class:text-gray-400={zone.mute}
						class:text-blue-600={!zone.mute}
						class:dark:text-blue-400={!zone.mute}
						aria-label={zone.mute ? `Unmute ${zone.name}` : `Mute ${zone.name}`}
						aria-pressed={!zone.mute}
					>
						<span class="text-2xl" aria-hidden="true">{zone.mute ? '🔇' : '🔊'}</span>
					</button>
				</div>

				<!-- Volume control -->
				<div class="space-y-2" role="group" aria-labelledby="vol-label-{zone.id}">
					<div class="flex items-center justify-between">
						<span id="vol-label-{zone.id}" class="text-sm font-medium text-gray-700 dark:text-gray-300">Volume</span>
						<span class="text-sm text-gray-600 dark:text-gray-400">
							{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}%
						</span>
					</div>

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
						class="w-full accent-blue-600"
						aria-label="Volume for {zone.name}"
						aria-valuemin={0}
						aria-valuemax={100}
						aria-valuenow={dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}
						aria-valuetext="{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)} percent"
					/>

					<div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
						<span>0%</span>
						<span>100%</span>
					</div>
				</div>

				<!-- Stream selector -->
				<div class="mt-4">
					<label for="zone-input-{zone.id}" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
						Input
					</label>
					<select
						id="zone-input-{zone.id}"
						value={zone.source_id >= 0 ? amplipi.getSource(zone.source_id)?.input || '' : 'none'}
						onchange={(e) => handleSourceInputChange(zone.id, e.currentTarget.value)}
						class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
					>
						<option value="none">None</option>
						{#each availableStreamsForZone(zone) as stream (stream.id)}
							<option value="stream={stream.id}">
								{stream.name}
								{#if stream.type === 'rca' && stream.config?.index !== undefined}
									(RCA Input {(stream.config.index as number) + 1})
								{/if}
							</option>
						{/each}
					</select>
				</div>
			</div>
		{/each}
	</div>

	{#if enabledZones.length === 0}
		<div
			class="rounded-lg border-2 border-dashed border-gray-300 p-12 text-center dark:border-gray-700"
		>
			<p class="text-gray-500 dark:text-gray-400">No zones available</p>
		</div>
	{/if}

	<!-- Zone availability -->
	<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
		<h3 class="text-sm font-semibold text-gray-900 dark:text-white">Zone Availability</h3>
		<p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
			Disabled zones are hidden from the controls above and their amplifiers stay switched off.
			Zones belonging to hardware detected after setup start disabled.
		</p>

		<div class="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
			{#each allZones as zone (zone.id)}
				<label
					class="flex items-center gap-3 rounded-lg border border-gray-200 p-3 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-700"
				>
					<input
						type="checkbox"
						checked={!zone.disabled}
						onchange={(e) => setZoneEnabled(zone, e.currentTarget.checked)}
						class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-2 focus:ring-blue-500 dark:border-gray-600"
					/>
					<span class="min-w-0 flex-1">
						<span
							class="block truncate text-sm font-medium text-gray-900 dark:text-white"
							class:text-gray-400={zone.disabled}
							class:dark:text-gray-500={zone.disabled}
						>
							{zone.name}
						</span>
						<span class="block text-xs text-gray-500 dark:text-gray-400">
							Zone {zone.id + 1}{zone.disabled ? ' · disabled' : ''}
						</span>
					</span>
				</label>
			{/each}
		</div>

		{#if allZones.length === 0}
			<p class="mt-4 text-sm text-gray-500 dark:text-gray-400">No zones detected.</p>
		{/if}
	</div>
</div>
