<script lang="ts">
	import { formatBytes, formatPercent } from '$lib/format';

	interface Props {
		label: string;
		detail?: string;
		usedBytes: number;
		totalBytes: number;
		/** Percent at which the meter is called out as nearly full. */
		warnAt?: number;
	}

	let { label, detail = '', usedBytes, totalBytes, warnAt = 85 }: Props = $props();

	const percent = $derived(totalBytes > 0 ? (usedBytes / totalBytes) * 100 : 0);
	const nearlyFull = $derived(percent >= warnAt);
</script>

<div class="space-y-1.5">
	<div class="flex items-baseline justify-between gap-3 text-sm">
		<div class="min-w-0">
			<span class="font-medium text-gray-900 dark:text-white">{label}</span>
			{#if detail}
				<span class="ml-2 text-xs text-gray-500 dark:text-gray-400">{detail}</span>
			{/if}
		</div>
		<span class="shrink-0 tabular-nums text-xs text-gray-600 dark:text-gray-400">
			{formatBytes(usedBytes)} / {formatBytes(totalBytes)}
		</span>
	</div>

	<div class="flex items-center gap-3">
		<div
			class="h-2 flex-1 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700"
			role="meter"
			aria-valuenow={Math.round(percent)}
			aria-valuemin="0"
			aria-valuemax="100"
			aria-label="{label} usage"
		>
			<!-- Rounded data-end anchored to the baseline, per mark spec -->
			<div class="meter-fill h-full rounded-full" style:width="{Math.min(100, percent)}%"></div>
		</div>
		<span
			class="w-20 shrink-0 text-right text-xs tabular-nums text-gray-600 dark:text-gray-400"
		>
			{formatPercent(percent)}{nearlyFull ? ' — full' : ''}
		</span>
	</div>
</div>

<style>
	/* Sequential blue: magnitude, not identity. */
	.meter-fill {
		background: #2a78d6;
	}

	@media (prefers-color-scheme: dark) {
		.meter-fill {
			background: #3987e5;
		}
	}
</style>
