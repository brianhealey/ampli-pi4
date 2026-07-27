<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Group, GroupUpdate } from '$lib/types';
	import { dbToPercent, percentToDb } from '$lib/volume';

	let showCreateDialog = $state(false);
	let newGroupName = $state('');
	let selectedZones = $state<number[]>([]);
	let dialogRef = $state<HTMLDivElement | null>(null);

	async function createGroup() {
		if (!newGroupName.trim()) return;

		try {
			await api.createGroup({ name: newGroupName, zones: selectedZones });
			showCreateDialog = false;
			newGroupName = '';
			selectedZones = [];
		} catch (err) {
			console.error('Failed to create group:', err);
		}
	}

	async function updateGroup(groupId: number, update: GroupUpdate) {
		try {
			await api.updateGroup(groupId, update);
		} catch (err) {
			console.error('Failed to update group:', err);
		}
	}

	async function deleteGroup(groupId: number) {
		if (!confirm('Delete this group?')) return;

		try {
			await api.deleteGroup(groupId);
		} catch (err) {
			console.error('Failed to delete group:', err);
		}
	}

	function toggleZoneSelection(zoneId: number) {
		if (selectedZones.includes(zoneId)) {
			selectedZones = selectedZones.filter((id) => id !== zoneId);
		} else {
			selectedZones = [...selectedZones, zoneId];
		}
	}

	async function updateZone(zoneId: number, update: { vol?: number; mute?: boolean }) {
		try {
			await api.updateZone(zoneId, update);
		} catch (err) {
			console.error('Failed to update zone:', err);
		}
	}

	function availableStreams() {
		return amplipi.streams.filter((stream) => !stream.disabled);
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

	function handleDialogKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			showCreateDialog = false;
			newGroupName = '';
			selectedZones = [];
		}
	}

	$effect(() => {
		if (showCreateDialog && dialogRef) {
			// Focus the first input when dialog opens
			const input = dialogRef.querySelector('input');
			if (input) input.focus();
		}
	});
</script>

<div class="p-4 md:p-6">
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Groups</h2>
			<p class="text-sm text-gray-600 dark:text-gray-400">Control multiple zones together</p>
		</div>

		<button
			onclick={() => (showCreateDialog = true)}
			class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
		>
			+ Create Group
		</button>
	</div>

	<!-- Groups grid -->
	<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
		{#each amplipi.groups as group (group.id)}
			{@const groupZones = amplipi.getGroupZones(group)}
			{@const source = group.source_id !== undefined ? amplipi.getSource(group.source_id) : null}

			<div
				class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800"
			>
				<!-- Group header -->
				<div class="mb-4 flex items-start justify-between">
					<div class="flex-1">
						<h3 class="font-semibold text-gray-900 dark:text-white">{group.name}</h3>
						<p class="text-sm text-gray-600 dark:text-gray-400">
							{groupZones.length} zone{groupZones.length !== 1 ? 's' : ''}
						</p>
						{#if source}
							<p class="text-xs text-gray-500 dark:text-gray-500">{source.name}</p>
						{/if}
					</div>

					<div class="flex gap-1">
						<button
							onclick={() => updateGroup(group.id, { mute: !(group.mute ?? false) })}
							class="rounded p-2 hover:bg-gray-100 dark:hover:bg-gray-700"
							class:text-gray-400={group.mute}
							class:text-blue-600={!group.mute}
							class:dark:text-blue-400={!group.mute}
							aria-label={group.mute ? `Unmute ${group.name}` : `Mute ${group.name}`}
							aria-pressed={!group.mute}
						>
							<span aria-hidden="true">{group.mute ? '🔇' : '🔊'}</span>
						</button>

						<button
							onclick={() => deleteGroup(group.id)}
							class="rounded p-2 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
							aria-label="Delete group {group.name}"
						>
							<span aria-hidden="true">🗑️</span>
						</button>
					</div>
				</div>

				<!-- Volume control -->
				{#if group.vol_f !== undefined}
					<div class="mb-3 space-y-2">
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium text-gray-700 dark:text-gray-300">Volume</span>
							<span class="text-sm text-gray-600 dark:text-gray-400">
								{Math.round((group.vol_f ?? 0) * 100)}%
							</span>
						</div>

						<input
							type="range"
							min="0"
							max="100"
							value={Math.round((group.vol_f ?? 0) * 100)}
							oninput={(e) => {
								const val = parseInt(e.currentTarget.value) / 100;
								updateGroup(group.id, { vol_f: val });
							}}
							class="w-full accent-blue-600"
							aria-label="Volume for {group.name}"
							aria-valuemin={0}
							aria-valuemax={100}
							aria-valuenow={Math.round((group.vol_f ?? 0) * 100)}
							aria-valuetext="{Math.round((group.vol_f ?? 0) * 100)} percent"
						/>
					</div>
				{/if}

				<!-- Individual zone controls -->
				<div class="space-y-3">
					<p class="text-xs font-medium text-gray-700 dark:text-gray-300">Individual Zones:</p>
					{#each groupZones as zone (zone.id)}
						<div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-600 dark:bg-gray-700/50">
							<!-- Zone header -->
							<div class="mb-2 flex items-center justify-between">
								<span class="text-sm font-medium text-gray-900 dark:text-white">{zone.name}</span>
								<button
									onclick={() => updateZone(zone.id, { mute: !zone.mute })}
									class="rounded p-1 hover:bg-gray-200 dark:hover:bg-gray-600"
									class:text-gray-400={zone.mute}
									class:text-blue-600={!zone.mute}
									class:dark:text-blue-400={!zone.mute}
									aria-label={zone.mute ? `Unmute ${zone.name}` : `Mute ${zone.name}`}
									aria-pressed={!zone.mute}
								>
									<span aria-hidden="true">{zone.mute ? '🔇' : '🔊'}</span>
								</button>
							</div>

							<!-- Zone volume control -->
							<div class="space-y-1">
								<div class="flex items-center justify-between">
									<span class="text-xs text-gray-600 dark:text-gray-400">Volume</span>
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

				<!-- Stream selector -->
				<div class="mt-3">
					<label for="group-stream-{group.id}" class="sr-only">Select input for {group.name}</label>
					<select
						id="group-stream-{group.id}"
						value={group.source_id !== undefined && group.source_id >= 0
							? amplipi.getSource(group.source_id)?.input || 'none'
							: 'none'}
						onchange={(e) => handleGroupInputChange(group.id, e.currentTarget.value)}
						class="w-full rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
					>
						<option value="none">No input</option>
						{#each availableStreams() as stream (stream.id)}
							<option value="stream={stream.id}">{stream.name}</option>
						{/each}
					</select>
				</div>
			</div>
		{/each}
	</div>

	{#if amplipi.groups.length === 0}
		<div
			class="rounded-lg border-2 border-dashed border-gray-300 p-12 text-center dark:border-gray-700"
		>
			<p class="mb-2 text-gray-500 dark:text-gray-400">No groups created yet</p>
			<button
				onclick={() => (showCreateDialog = true)}
				class="text-sm text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
			>
				Create your first group
			</button>
		</div>
	{/if}
</div>

<!-- Create group dialog -->
{#if showCreateDialog}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onclick={(e) => {
			if (e.target === e.currentTarget) showCreateDialog = false;
		}}
		onkeydown={handleDialogKeydown}
		role="dialog"
		aria-modal="true"
		aria-labelledby="create-group-title"
		tabindex="-1"
	>
		<div
			bind:this={dialogRef}
			class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-gray-800"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 id="create-group-title" class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">Create Group</h3>

			<div class="mb-4">
				<label for="new-group-name" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
					Group Name
				</label>
				<input
					id="new-group-name"
					type="text"
					bind:value={newGroupName}
					placeholder="e.g., Living Room"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
				/>
			</div>

			<div class="mb-4">
				<span id="zone-selection-label" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
					Select Zones
				</span>
				<div class="max-h-60 space-y-1 overflow-y-auto" role="group" aria-labelledby="zone-selection-label">
					{#each amplipi.zones.filter((z) => !z.disabled) as zone (zone.id)}
						<button
							onclick={() => toggleZoneSelection(zone.id)}
							class={`flex w-full items-center gap-2 rounded-lg border px-3 py-2 text-left transition-colors ${selectedZones.includes(zone.id) ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20' : 'border-gray-300 dark:border-gray-600'}`}
							role="checkbox"
							aria-checked={selectedZones.includes(zone.id)}
						>
							<div
								class={`flex h-5 w-5 items-center justify-center rounded border-2 ${selectedZones.includes(zone.id) ? 'border-blue-600 bg-blue-600' : 'border-gray-300 dark:border-gray-600'}`}
								aria-hidden="true"
							>
								{#if selectedZones.includes(zone.id)}
									<span class="text-white">✓</span>
								{/if}
							</div>
							<span class="text-sm text-gray-900 dark:text-white">{zone.name}</span>
						</button>
					{/each}
				</div>
			</div>

			<div class="flex gap-2">
				<button
					onclick={() => {
						showCreateDialog = false;
						newGroupName = '';
						selectedZones = [];
					}}
					class="flex-1 rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
				>
					Cancel
				</button>
				<button
					onclick={createGroup}
					disabled={!newGroupName.trim() || selectedZones.length === 0}
					class="flex-1 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
				>
					Create
				</button>
			</div>
		</div>
	</div>
{/if}
