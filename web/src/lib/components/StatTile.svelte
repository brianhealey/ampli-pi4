<script lang="ts">
	interface Props {
		label: string;
		value: string;
		detail?: string;
		icon?: string;
	}

	let { label, value, detail = '', icon = '' }: Props = $props();
</script>

<!-- A headline number: no plot, so no hover layer — the value is the content. -->
<div
	class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800"
>
	<div class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
		{#if icon}<span aria-hidden="true">{icon}</span>{/if}
		<span>{label}</span>
	</div>
	<div
		class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white"
	>
		{value}
	</div>
	{#if detail}
		<div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{detail}</div>
	{/if}
</div>
