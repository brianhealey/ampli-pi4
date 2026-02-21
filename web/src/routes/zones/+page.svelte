<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Zone } from '$lib/types';

	async function updateZone(zoneId: number, update: Partial<Zone>) {
		try {
			await api.updateZone(zoneId, update);
		} catch (err) {
			console.error('Failed to update zone:', err);
		}
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
		<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Zones</h2>
		<p class="text-sm text-gray-600 dark:text-gray-400">Manage all audio zones</p>
	</div>

	<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
		{#each amplipi.zones.filter((z) => !z.disabled) as zone (zone.id)}
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
					>
						<span class="text-2xl">{zone.mute ? '🔇' : '🔊'}</span>
					</button>
				</div>

				<!-- Volume control -->
				<div class="space-y-2">
					<div class="flex items-center justify-between">
						<span class="text-sm font-medium text-gray-700 dark:text-gray-300">Volume</span>
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
					/>

					<div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
						<span>0%</span>
						<span>100%</span>
					</div>
				</div>

				<!-- Source selector -->
				<div class="mt-4">
					<label class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
						Source
					</label>
					<select
						value={zone.source_id}
						onchange={(e) => updateZone(zone.id, { source_id: parseInt(e.currentTarget.value) })}
						class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
					>
						<option value={-1}>None</option>
						{#each amplipi.sources as src (src.id)}
							<option value={src.id}>{src.name}</option>
						{/each}
					</select>
				</div>
			</div>
		{/each}
	</div>

	{#if amplipi.zones.filter((z) => !z.disabled).length === 0}
		<div
			class="rounded-lg border-2 border-dashed border-gray-300 p-12 text-center dark:border-gray-700"
		>
			<p class="text-gray-500 dark:text-gray-400">No zones available</p>
		</div>
	{/if}
</div>
