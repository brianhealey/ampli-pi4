<script lang="ts">
	import { amplipi } from '$lib/store.svelte';
	import { api } from '$lib/api';
	import type { Stream } from '$lib/types';

	let showCreateDialog = $state(false);
	let newStreamName = $state('');
	let newStreamType = $state('spotify');
	let newStreamConfig = $state<Record<string, string>>({});

	const streamTypes = [
		{ value: 'spotify', label: 'Spotify Connect', icon: '🎵' },
		{ value: 'airplay', label: 'AirPlay', icon: '📡' },
		{ value: 'pandora', label: 'Pandora', icon: '🎙️' },
		{ value: 'internetradio', label: 'Internet Radio', icon: '📻' },
		{ value: 'lms', label: 'Logitech Media Server', icon: '🔊' },
		{ value: 'dlna', label: 'DLNA/UPnP', icon: '🌐' }
	];

	async function createStream() {
		if (!newStreamName.trim()) return;

		try {
			const config: Record<string, unknown> = { ...newStreamConfig };

			// Add type-specific defaults
			if (newStreamType === 'internetradio' && !config.url) {
				alert('URL is required for Internet Radio');
				return;
			}

			await api.createStream({
				name: newStreamName,
				type: newStreamType,
				config
			});

			showCreateDialog = false;
			newStreamName = '';
			newStreamType = 'spotify';
			newStreamConfig = {};
		} catch (err) {
			console.error('Failed to create stream:', err);
			alert('Failed to create stream: ' + (err as Error).message);
		}
	}

	async function execCommand(streamId: number, command: string) {
		try {
			await api.execStreamCommand(streamId, command);
		} catch (err) {
			console.error('Failed to execute command:', err);
		}
	}

	async function deleteStream(streamId: number) {
		if (!confirm('Delete this stream?')) return;

		try {
			await api.deleteStream(streamId);
		} catch (err) {
			console.error('Failed to delete stream:', err);
		}
	}

	function getStreamIcon(type: string): string {
		const icons: Record<string, string> = {
			spotify: '🎵',
			airplay: '📡',
			pandora: '🎙️',
			internetradio: '📻',
			lms: '🔊',
			dlna: '🌐',
			bluetooth: '📱',
			rca: '🔌',
			aux: '🎧',
			fm_radio: '📻',
			file_player: '📁'
		};
		return icons[type] || '🎵';
	}

	function getStatusColor(state: string): string {
		switch (state) {
			case 'playing':
				return 'text-green-600 dark:text-green-400';
			case 'paused':
				return 'text-yellow-600 dark:text-yellow-400';
			case 'stopped':
				return 'text-gray-600 dark:text-gray-400';
			default:
				return 'text-gray-500 dark:text-gray-500';
		}
	}
</script>

<div class="p-4 md:p-6">
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Streams</h2>
			<p class="text-sm text-gray-600 dark:text-gray-400">Manage audio streaming services</p>
		</div>

		<button
			onclick={() => (showCreateDialog = true)}
			class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
		>
			+ Create Stream
		</button>
	</div>

	<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
		{#each amplipi.streams as stream (stream.id)}
			<div
				class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800"
			>
				<!-- Stream header -->
				<div class="mb-3 flex items-start justify-between">
					<div class="flex items-center gap-2">
						<span class="text-2xl">{getStreamIcon(stream.type)}</span>
						<div>
							<h3 class="font-semibold text-gray-900 dark:text-white">{stream.name}</h3>
							<p class="text-xs text-gray-600 dark:text-gray-400">{stream.type}</p>
						</div>
					</div>

					{#if stream.type !== 'rca' && stream.type !== 'aux'}
						<button
							onclick={() => deleteStream(stream.id)}
							class="rounded p-1 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
						>
							🗑️
						</button>
					{/if}
				</div>

				<!-- Stream info -->
				{#if stream.info}
					<div class="mb-3 rounded-lg bg-gray-50 p-3 dark:bg-gray-700/50">
						{#if stream.info.state}
							<div class="mb-1 flex items-center justify-between">
								<span class="text-xs font-medium text-gray-700 dark:text-gray-300">Status</span>
								<span class="text-xs {getStatusColor(stream.info.state)}">
									{stream.info.state}
								</span>
							</div>
						{/if}

						{#if stream.info.track}
							<p class="mb-1 text-sm font-medium text-gray-900 dark:text-white">
								{stream.info.track}
							</p>
						{/if}

						{#if stream.info.artist}
							<p class="text-xs text-gray-600 dark:text-gray-400">
								{stream.info.artist}
								{#if stream.info.album} • {stream.info.album}{/if}
							</p>
						{/if}

						{#if stream.info.station}
							<p class="text-xs text-gray-600 dark:text-gray-400">{stream.info.station}</p>
						{/if}
					</div>
				{/if}

				<!-- Playback controls -->
				{#if stream.info?.state && ['playing', 'paused'].includes(stream.info.state)}
					<div class="flex items-center justify-center gap-2">
						<button
							onclick={() => execCommand(stream.id, 'prev')}
							class="rounded-lg p-2 hover:bg-gray-100 dark:hover:bg-gray-700"
							title="Previous"
						>
							⏮️
						</button>

						{#if stream.info.state === 'playing'}
							<button
								onclick={() => execCommand(stream.id, 'pause')}
								class="rounded-lg bg-gray-100 p-3 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
								title="Pause"
							>
								⏸️
							</button>
						{:else}
							<button
								onclick={() => execCommand(stream.id, 'play')}
								class="rounded-lg bg-blue-100 p-3 hover:bg-blue-200 dark:bg-blue-900/20 dark:hover:bg-blue-900/30"
								title="Play"
							>
								▶️
							</button>
						{/if}

						<button
							onclick={() => execCommand(stream.id, 'next')}
							class="rounded-lg p-2 hover:bg-gray-100 dark:hover:bg-gray-700"
							title="Next"
						>
							⏭️
						</button>

						<button
							onclick={() => execCommand(stream.id, 'stop')}
							class="rounded-lg p-2 hover:bg-gray-100 dark:hover:bg-gray-700"
							title="Stop"
						>
							⏹️
						</button>
					</div>
				{/if}

				<!-- Disabled indicator -->
				{#if stream.disabled}
					<div class="mt-2 rounded bg-yellow-50 px-2 py-1 text-center text-xs text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-400">
						Disabled
					</div>
				{/if}
			</div>
		{/each}
	</div>

	{#if amplipi.streams.length === 0}
		<div
			class="rounded-lg border-2 border-dashed border-gray-300 p-12 text-center dark:border-gray-700"
		>
			<p class="text-gray-500 dark:text-gray-400">No streams available</p>
		</div>
	{/if}
</div>

<!-- Create stream dialog -->
{#if showCreateDialog}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		role="dialog"
		aria-modal="true"
		onclick={(e) => {
			if (e.target === e.currentTarget) showCreateDialog = false;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') showCreateDialog = false;
		}}
	>
		<div
			class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-gray-800"
			role="document"
			onclick={(e) => e.stopPropagation()}
		>
			<h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">Create Stream</h3>

			<div class="mb-4">
				<label for="stream-name" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
					Stream Name
				</label>
				<input
					id="stream-name"
					type="text"
					bind:value={newStreamName}
					placeholder="e.g., My Spotify"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
				/>
			</div>

			<div class="mb-4">
				<label for="stream-type" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
					Stream Type
				</label>
				<select
					id="stream-type"
					bind:value={newStreamType}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
				>
					{#each streamTypes as type}
						<option value={type.value}>{type.icon} {type.label}</option>
					{/each}
				</select>
			</div>

			<!-- Type-specific fields -->
			{#if newStreamType === 'internetradio'}
				<div class="mb-4">
					<label for="stream-url" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">
						Stream URL
					</label>
					<input
						id="stream-url"
						type="url"
						bind:value={newStreamConfig.url}
						placeholder="http://example.com/stream.mp3"
						class="w-full rounded-lg border border-gray-300 px-3 py-2 focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
					/>
				</div>
			{/if}

			<div class="flex gap-2">
				<button
					onclick={() => {
						showCreateDialog = false;
						newStreamName = '';
						newStreamType = 'spotify';
						newStreamConfig = {};
					}}
					class="flex-1 rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
				>
					Cancel
				</button>
				<button
					onclick={createStream}
					disabled={!newStreamName.trim()}
					class="flex-1 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
				>
					Create
				</button>
			</div>
		</div>
	</div>
{/if}
