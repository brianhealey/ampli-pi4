<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Zone, Group, Source } from '$lib/types';
	import { dbToPercent, percentToDb, volFToPercent } from '$lib/volume';

	let editingZone = $state<number | null>(null);
	let editingGroup = $state<number | null>(null);
	let editingName = $state('');
	let expanded = $state<Set<number>>(new Set());

	const groups = $derived([...amplipi.groups].sort((a, b) => a.id - b.id));

	// A zone in a group is represented by that group, not as a row of its own.
	const groupedZoneIds = $derived(new Set(amplipi.groups.flatMap((g) => g.zones)));

	// Everything else, in hardware order. Zones on a unit added after the config
	// was written arrive disabled, so this page is where they get switched on.
	const ungrouped = $derived(
		[...amplipi.zones].filter((z) => !groupedZoneIds.has(z.id)).sort((a, b) => a.id - b.id)
	);

	function membersOf(group: Group): Zone[] {
		return group.zones
			.map((id) => amplipi.getZone(id))
			.filter((z): z is Zone => z !== undefined)
			.sort((a, b) => a.id - b.id);
	}

	function sourceLabel(src: Source): string {
		if (!src.input) return `${src.name} — no input`;
		if (src.input.startsWith('stream=')) {
			const stream = amplipi.getStream(parseInt(src.input.substring(7)));
			return `${src.name} — ${stream?.name ?? src.input}`;
		}
		return `${src.name} — ${src.input}`;
	}

	// The source a group is on, taken from its members. Shows blank if they
	// disagree, which can happen if zones were moved individually.
	function groupSource(group: Group): string {
		const ids = new Set(membersOf(group).map((z) => z.source_id));
		if (ids.size !== 1) return '';
		const only = [...ids][0];
		return only >= 0 ? String(only) : 'none';
	}

	function toggleExpanded(id: number) {
		const next = new Set(expanded);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		expanded = next;
	}

	async function updateZone(zoneId: number, patch: Parameters<typeof api.updateZone>[1]) {
		try {
			await api.updateZone(zoneId, patch);
		} catch (err) {
			console.error('Failed to update zone:', err);
		}
	}

	async function updateGroup(groupId: number, patch: Parameters<typeof api.updateGroup>[1]) {
		try {
			await api.updateGroup(groupId, patch);
		} catch (err) {
			console.error('Failed to update group:', err);
		}
	}

	// Disabling also mutes, so a zone can never be left driving a speaker while
	// hidden from the controls on Home.
	async function setZoneEnabled(zone: Zone, enabled: boolean) {
		await updateZone(zone.id, enabled ? { disabled: false } : { disabled: true, mute: true });
	}

	// Assigning to a source is the only routing decision. It does not touch the
	// source's input — that belongs to the source.
	async function setZoneSource(zone: Zone, value: string) {
		await updateZone(zone.id, { source_id: value === 'none' ? -1 : parseInt(value) });
	}

	// The controller propagates a group's source to every member zone.
	async function setGroupSource(group: Group, value: string) {
		await updateGroup(group.id, { source_id: value === 'none' ? -1 : parseInt(value) });
	}

	async function setGroupVolume(group: Group, percent: number) {
		const delta = Math.round(((percent - volFToPercent(group.vol_f ?? 0)) / 100) * 79);
		if (delta !== 0) await updateGroup(group.id, { vol_delta: delta });
	}

	async function commitZoneName(zoneId: number) {
		const name = editingName.trim();
		editingZone = null;
		if (name) await updateZone(zoneId, { name });
	}

	async function commitGroupName(groupId: number) {
		const name = editingName.trim();
		editingGroup = null;
		if (name) await updateGroup(groupId, { name });
	}
</script>

<div class="p-4 md:p-6">
	<div class="mb-6">
		<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Zones</h2>
		<p class="text-sm text-gray-600 dark:text-gray-400">
			Each zone listens to one source. Grouped zones are managed through their group. Disabled
			zones are hidden from Home and their amplifiers stay switched off.
		</p>
	</div>

	<div class="space-y-3">
		<!-- Groups stand in for their member zones -->
		{#each groups as group (group.id)}
			{@const members = membersOf(group)}
			{@const isOpen = expanded.has(group.id)}

			<div class="rounded-lg border border-purple-200 bg-white p-4 dark:border-purple-800 dark:bg-gray-800">
				<div class="flex flex-wrap items-center gap-3">
					<button
						onclick={() => toggleExpanded(group.id)}
						class="shrink-0 text-sm text-purple-700 dark:text-purple-400"
						aria-expanded={isOpen}
						aria-label={isOpen ? `Collapse ${group.name}` : `Expand ${group.name}`}
					>
						{isOpen ? '▼' : '▶'}
					</button>

					<div class="min-w-0 flex-1">
						{#if editingGroup === group.id}
							<!-- svelte-ignore a11y_autofocus -->
							<input
								type="text"
								bind:value={editingName}
								autofocus
								onblur={() => commitGroupName(group.id)}
								onkeydown={(e) => {
									if (e.key === 'Enter') commitGroupName(group.id);
									if (e.key === 'Escape') editingGroup = null;
								}}
								class="w-full rounded border border-blue-500 bg-white px-2 py-1 text-sm font-semibold dark:bg-gray-700 dark:text-white"
								aria-label="Group name"
							/>
						{:else}
							<button
								onclick={() => {
									editingGroup = group.id;
									editingName = group.name;
								}}
								class="truncate text-left text-sm font-semibold text-purple-800 hover:underline dark:text-purple-300"
								title="Rename group"
							>
								{group.name}
							</button>
						{/if}
						<span class="block text-xs text-purple-600 dark:text-purple-400">
							Group · {members.length} zone{members.length === 1 ? '' : 's'}
						</span>
					</div>

					<select
						value={groupSource(group)}
						onchange={(e) => setGroupSource(group, e.currentTarget.value)}
						class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
						aria-label="Source for {group.name}"
					>
						<option value="">Mixed</option>
						<option value="none">No source</option>
						{#each amplipi.sources as src (src.id)}
							<option value={String(src.id)}>{sourceLabel(src)}</option>
						{/each}
					</select>

					<button
						onclick={() => updateGroup(group.id, { mute: !group.mute })}
						class="rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-700"
						aria-label={group.mute ? `Unmute ${group.name}` : `Mute ${group.name}`}
						aria-pressed={!group.mute}
					>
						<span class="text-lg" aria-hidden="true">{group.mute ? '🔇' : '🔊'}</span>
					</button>
				</div>

				<div class="mt-3 flex items-center gap-3">
					<input
						type="range"
						min="0"
						max="100"
						value={volFToPercent(group.vol_f ?? 0)}
						onchange={(e) => setGroupVolume(group, parseInt(e.currentTarget.value))}
						class="w-full"
						aria-label="Volume for {group.name}"
					/>
					<span class="w-10 shrink-0 text-right text-xs text-gray-600 dark:text-gray-400">
						{volFToPercent(group.vol_f ?? 0)}%
					</span>
				</div>

				<!-- Member zones: per-zone settings that a group cannot express -->
				{#if isOpen}
					<div class="mt-4 space-y-3 border-t border-purple-200 pt-3 dark:border-purple-800">
						{#each members as zone (zone.id)}
							<div class="flex flex-wrap items-center gap-3 rounded-lg border border-gray-200 p-3 dark:border-gray-700">
								<label class="flex shrink-0 items-center gap-2">
									<input
										type="checkbox"
										checked={!zone.disabled}
										onchange={(e) => setZoneEnabled(zone, e.currentTarget.checked)}
										class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-2 focus:ring-blue-500 dark:border-gray-600"
										aria-label="Enable {zone.name}"
									/>
									<span class="text-xs text-gray-500 dark:text-gray-400">Zone {zone.id + 1}</span>
								</label>

								<div class="min-w-0 flex-1">
									{#if editingZone === zone.id}
										<!-- svelte-ignore a11y_autofocus -->
										<input
											type="text"
											bind:value={editingName}
											autofocus
											onblur={() => commitZoneName(zone.id)}
											onkeydown={(e) => {
												if (e.key === 'Enter') commitZoneName(zone.id);
												if (e.key === 'Escape') editingZone = null;
											}}
											class="w-full rounded border border-blue-500 bg-white px-2 py-1 text-sm dark:bg-gray-700 dark:text-white"
											aria-label="Zone name"
										/>
									{:else}
										<button
											onclick={() => {
												editingZone = zone.id;
												editingName = zone.name;
											}}
											class="truncate text-left text-sm font-medium hover:underline"
											class:text-gray-900={!zone.disabled}
											class:dark:text-white={!zone.disabled}
											class:text-gray-400={zone.disabled}
											class:dark:text-gray-500={zone.disabled}
											title="Rename zone"
										>
											{zone.name}
										</button>
									{/if}
								</div>

								<input
									type="range"
									min="0"
									max="100"
									value={dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}
									disabled={zone.disabled}
									onchange={(e) =>
										updateZone(zone.id, {
											vol: percentToDb(parseInt(e.currentTarget.value), zone.vol_min, zone.vol_max)
										})}
									class="w-full min-w-24 flex-1 disabled:opacity-40"
									aria-label="Volume for {zone.name}"
								/>

								<button
									onclick={() => updateZone(zone.id, { mute: !zone.mute })}
									disabled={zone.disabled}
									class="rounded p-1 hover:bg-gray-100 disabled:opacity-40 dark:hover:bg-gray-700"
									aria-label={zone.mute ? `Unmute ${zone.name}` : `Mute ${zone.name}`}
									aria-pressed={!zone.mute}
								>
									<span class="text-lg" aria-hidden="true">{zone.mute ? '🔇' : '🔊'}</span>
								</button>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/each}

		<!-- Zones not in any group -->
		{#each ungrouped as zone (zone.id)}
			<div
				class="rounded-lg border bg-white p-4 dark:bg-gray-800"
				class:border-gray-200={!zone.disabled}
				class:dark:border-gray-700={!zone.disabled}
				class:border-dashed={zone.disabled}
				class:border-gray-300={zone.disabled}
				class:dark:border-gray-600={zone.disabled}
			>
				<div class="flex flex-wrap items-center gap-3">
					<label class="flex shrink-0 items-center gap-2">
						<input
							type="checkbox"
							checked={!zone.disabled}
							onchange={(e) => setZoneEnabled(zone, e.currentTarget.checked)}
							class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-2 focus:ring-blue-500 dark:border-gray-600"
							aria-label="Enable {zone.name}"
						/>
						<span class="text-xs text-gray-500 dark:text-gray-400">Zone {zone.id + 1}</span>
					</label>

					<div class="min-w-0 flex-1">
						{#if editingZone === zone.id}
							<!-- svelte-ignore a11y_autofocus -->
							<input
								type="text"
								bind:value={editingName}
								autofocus
								onblur={() => commitZoneName(zone.id)}
								onkeydown={(e) => {
									if (e.key === 'Enter') commitZoneName(zone.id);
									if (e.key === 'Escape') editingZone = null;
								}}
								class="w-full rounded border border-blue-500 bg-white px-2 py-1 text-sm font-semibold dark:bg-gray-700 dark:text-white"
								aria-label="Zone name"
							/>
						{:else}
							<button
								onclick={() => {
									editingZone = zone.id;
									editingName = zone.name;
								}}
								class="truncate text-left text-sm font-semibold hover:underline"
								class:text-gray-900={!zone.disabled}
								class:dark:text-white={!zone.disabled}
								class:text-gray-400={zone.disabled}
								class:dark:text-gray-500={zone.disabled}
								title="Rename zone"
							>
								{zone.name}
							</button>
						{/if}
					</div>

					<select
						value={zone.source_id >= 0 ? String(zone.source_id) : 'none'}
						onchange={(e) => setZoneSource(zone, e.currentTarget.value)}
						disabled={zone.disabled}
						class="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 disabled:opacity-50 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
						aria-label="Source for {zone.name}"
					>
						<option value="none">No source</option>
						{#each amplipi.sources as src (src.id)}
							<option value={String(src.id)}>{sourceLabel(src)}</option>
						{/each}
					</select>

					<button
						onclick={() => updateZone(zone.id, { mute: !zone.mute })}
						disabled={zone.disabled}
						class="rounded p-1 hover:bg-gray-100 disabled:opacity-40 dark:hover:bg-gray-700"
						aria-label={zone.mute ? `Unmute ${zone.name}` : `Mute ${zone.name}`}
						aria-pressed={!zone.mute}
					>
						<span class="text-lg" aria-hidden="true">{zone.mute ? '🔇' : '🔊'}</span>
					</button>
				</div>

				<div class="mt-3 flex items-center gap-3">
					<input
						type="range"
						min="0"
						max="100"
						value={dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}
						disabled={zone.disabled}
						onchange={(e) =>
							updateZone(zone.id, {
								vol: percentToDb(parseInt(e.currentTarget.value), zone.vol_min, zone.vol_max)
							})}
						class="w-full disabled:opacity-40"
						aria-label="Volume for {zone.name}"
					/>
					<span class="w-10 shrink-0 text-right text-xs text-gray-600 dark:text-gray-400">
						{dbToPercent(zone.vol, zone.vol_min, zone.vol_max)}%
					</span>
				</div>
			</div>
		{/each}
	</div>

	{#if groups.length === 0 && ungrouped.length === 0}
		<div
			class="rounded-lg border-2 border-dashed border-gray-300 p-12 text-center dark:border-gray-700"
		>
			<p class="text-gray-500 dark:text-gray-400">No zones detected</p>
		</div>
	{/if}
</div>
