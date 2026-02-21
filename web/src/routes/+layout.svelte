<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { amplipi } from '$lib/store.svelte';

	let { children } = $props();

	// Navigation items
	const navItems = [
		{ path: '/', label: 'Home', icon: '🏠' },
		{ path: '/zones', label: 'Zones', icon: '🔊' },
		{ path: '/groups', label: 'Groups', icon: '👥' },
		{ path: '/streams', label: 'Streams', icon: '🎵' }
	];

	// Cleanup on unmount
	$effect(() => {
		return () => {
			amplipi.stopPolling();
		};
	});
</script>

<div class="flex h-screen flex-col bg-gray-50 dark:bg-gray-900">
	<!-- Header -->
	<header
		class="flex items-center justify-between border-b border-gray-200 bg-white px-4 py-3 shadow-sm dark:border-gray-700 dark:bg-gray-800"
	>
		<div class="flex items-center gap-3">
			<div class="text-2xl">🎛️</div>
			<h1 class="text-xl font-bold text-gray-900 dark:text-white">AmpliPi</h1>
		</div>

		<!-- Connection status -->
		<div class="flex items-center gap-2">
			{#if amplipi.loading}
				<div class="h-2 w-2 animate-pulse rounded-full bg-yellow-500"></div>
				<span class="text-sm text-gray-600 dark:text-gray-400">Loading...</span>
			{:else if !amplipi.connected}
				<div class="h-2 w-2 rounded-full bg-red-500"></div>
				<span class="text-sm text-red-600 dark:text-red-400">Disconnected</span>
			{:else}
				<div class="h-2 w-2 rounded-full bg-green-500"></div>
				<span class="text-sm text-gray-600 dark:text-gray-400">Connected</span>
			{/if}
		</div>
	</header>

	<!-- Main content area -->
	<div class="flex flex-1 overflow-hidden">
		<!-- Desktop sidebar navigation -->
		<nav
			class="hidden w-64 border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800 md:block"
		>
			<div class="space-y-1 p-4">
				{#each navItems as item}
					<a
						href={item.path}
						class={`flex items-center gap-3 rounded-lg px-4 py-3 text-sm font-medium transition-colors hover:bg-gray-100 dark:hover:bg-gray-700 ${$page.url.pathname === item.path ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400' : 'text-gray-700 dark:text-gray-300'}`}
					>
						<span class="text-xl">{item.icon}</span>
						<span>{item.label}</span>
					</a>
				{/each}
			</div>
		</nav>

		<!-- Page content -->
		<main class="flex-1 overflow-y-auto pb-20 md:pb-0">
			{#if amplipi.error}
				<div class="m-4 rounded-lg bg-red-50 p-4 dark:bg-red-900/20">
					<div class="flex items-center gap-2">
						<span class="text-xl">⚠️</span>
						<div>
							<h3 class="font-semibold text-red-800 dark:text-red-200">Connection Error</h3>
							<p class="text-sm text-red-600 dark:text-red-400">{amplipi.error}</p>
						</div>
					</div>
				</div>
			{/if}

			{@render children()}
		</main>
	</div>

	<!-- Mobile bottom navigation -->
	<nav
		class="fixed bottom-0 left-0 right-0 border-t border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800 md:hidden"
	>
		<div class="flex items-center justify-around">
			{#each navItems as item}
				<a
					href={item.path}
					class="flex flex-1 flex-col items-center gap-1 py-2 transition-colors"
					class:text-blue-600={$page.url.pathname === item.path}
					class:dark:text-blue-400={$page.url.pathname === item.path}
					class:text-gray-600={$page.url.pathname !== item.path}
					class:dark:text-gray-400={$page.url.pathname !== item.path}
				>
					<span class="text-2xl">{item.icon}</span>
					<span class="text-xs font-medium">{item.label}</span>
				</a>
			{/each}
		</div>
	</nav>
</div>
