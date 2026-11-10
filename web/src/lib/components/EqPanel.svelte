<script lang="ts">
	import { api, apiErrorMessage } from '$lib/api';
	import type { EQBand, EQBandType, Source, SourceEQ } from '$lib/types';

	interface Props {
		source: Source;
		// CamillaDSP is installed on this unit.
		available: boolean;
	}

	let { source, available }: Props = $props();

	const MAX_BANDS = 10;
	// Wait this long after the last change before sending, so dragging a
	// slider does not flood the API (the backend applies each change live).
	const SEND_DELAY_MS = 300;
	// After a send, ignore polled state this long so an older poll does not
	// snap the controls back.
	const SETTLE_MS = 2500;

	const flat = (): SourceEQ => ({ enabled: false, preamp_db: 0, bands: [] });

	function clone(eq: SourceEQ): SourceEQ {
		return {
			enabled: eq.enabled,
			preamp_db: eq.preamp_db,
			bands: (eq.bands ?? []).map((b) => ({ ...b }))
		};
	}

	// Presets: the preamp equals minus the largest boost, so none can clip.
	const presets: { name: string; preamp_db: number; bands: EQBand[] }[] = [
		{ name: 'Flat', preamp_db: 0, bands: [] },
		{
			name: 'Bass boost',
			preamp_db: -6,
			bands: [{ type: 'lowshelf', freq: 100, gain: 6, q: 0.7 }]
		},
		{
			name: 'Treble boost',
			preamp_db: -5,
			bands: [{ type: 'highshelf', freq: 8000, gain: 5, q: 0.7 }]
		},
		{
			name: 'Vocal clarity',
			preamp_db: -3,
			bands: [
				{ type: 'peaking', freq: 250, gain: -2, q: 1 },
				{ type: 'peaking', freq: 3000, gain: 3, q: 1.2 },
				{ type: 'peaking', freq: 5000, gain: 2, q: 1.5 }
			]
		},
		{
			name: 'Loudness',
			preamp_db: -6,
			bands: [
				{ type: 'lowshelf', freq: 80, gain: 6, q: 0.7 },
				{ type: 'highshelf', freq: 10000, gain: 4, q: 0.7 }
			]
		}
	];

	let draft = $state<SourceEQ>(flat());
	let error = $state<string | null>(null);

	// Plain variables: timing bookkeeping must not drive reactivity.
	let sendTimer: ReturnType<typeof setTimeout> | null = null;
	let settleUntil = 0;

	// Follow the server (polled every 2 s) while the user is not editing.
	$effect(() => {
		const server = source.eq;
		if (sendTimer === null && Date.now() >= settleUntil) {
			draft = clone(server ?? flat());
		}
	});

	$effect(() => {
		return () => {
			if (sendTimer !== null) clearTimeout(sendTimer);
		};
	});

	function valid(eq: SourceEQ): string | null {
		if (eq.preamp_db < -24 || eq.preamp_db > 0) return 'Preamp must be between -24 and 0 dB.';
		for (const [i, b] of eq.bands.entries()) {
			const n = i + 1;
			if (!(b.freq >= 20 && b.freq <= 20000)) return `Band ${n}: frequency must be 20-20000 Hz.`;
			if (!(b.gain >= -12 && b.gain <= 12)) return `Band ${n}: gain must be -12 to +12 dB.`;
			if (!(b.q >= 0.1 && b.q <= 10)) return `Band ${n}: Q must be 0.1-10.`;
		}
		return null;
	}

	function scheduleSend() {
		if (sendTimer !== null) clearTimeout(sendTimer);
		settleUntil = Date.now() + SETTLE_MS;
		const invalid = valid(draft);
		if (invalid) {
			error = invalid;
			sendTimer = null;
			return;
		}
		sendTimer = setTimeout(send, SEND_DELAY_MS);
	}

	async function send() {
		sendTimer = null;
		settleUntil = Date.now() + SETTLE_MS;
		try {
			await api.updateSource(source.id, { eq: clone(draft) });
			error = null;
		} catch (err) {
			error = apiErrorMessage(err);
		}
	}

	function applyPreset(p: (typeof presets)[number]) {
		draft = {
			enabled: p.name === 'Flat' ? draft.enabled : true,
			preamp_db: p.preamp_db,
			bands: p.bands.map((b) => ({ ...b }))
		};
		scheduleSend();
	}

	function addBand() {
		if (draft.bands.length >= MAX_BANDS) return;
		draft.bands.push({ type: 'peaking', freq: 1000, gain: 0, q: 1 });
		scheduleSend();
	}

	function removeBand(i: number) {
		draft.bands.splice(i, 1);
		scheduleSend();
	}

	function setNumber(apply: (v: number) => void, raw: string) {
		const v = parseFloat(raw);
		if (Number.isNaN(v)) return;
		apply(v);
		scheduleSend();
	}

	const maxBoost = $derived(Math.max(0, ...draft.bands.map((b) => b.gain)));
	const mayClip = $derived(draft.enabled && draft.preamp_db + maxBoost > 0);

	// What is actually happening, in one line.
	const status = $derived.by((): { text: string; tone: 'ok' | 'muted' | 'warn' | 'bad' } => {
		if (!available)
			return { text: 'Unavailable on this unit (CamillaDSP is not installed)', tone: 'muted' };
		const st = source.eq_status;
		if (!st || !source.eq?.enabled) return { text: 'Off', tone: 'muted' };
		switch (st.state) {
			case 'active':
				return { text: 'Active', tone: 'ok' };
			case 'starting':
				return { text: 'Starting…', tone: 'muted' };
			case 'idle':
				return { text: 'On; no stream playing on this source', tone: 'muted' };
			case 'analog':
				return { text: 'Not applied: analog inputs (RCA/Aux) bypass the EQ', tone: 'warn' };
			case 'no_dac':
				return { text: 'Not applied: this source has no DAC', tone: 'warn' };
			case 'unavailable':
				return { text: 'Unavailable on this unit', tone: 'muted' };
			case 'failed':
				return { text: `Failed: ${st.reason ?? 'unknown error'}`, tone: 'bad' };
			default:
				return { text: 'Off', tone: 'muted' };
		}
	});

	const toneClass = {
		ok: 'text-green-700 dark:text-green-400',
		muted: 'text-gray-500 dark:text-gray-400',
		warn: 'text-amber-700 dark:text-amber-400',
		bad: 'text-red-600 dark:text-red-400'
	};

	const bandTypes: { value: EQBandType; label: string }[] = [
		{ value: 'peaking', label: 'Peak' },
		{ value: 'lowshelf', label: 'Low shelf' },
		{ value: 'highshelf', label: 'High shelf' }
	];

	const inputClass =
		'w-full rounded border border-gray-300 bg-white px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white';
</script>

<details class="mt-3 rounded-lg border border-gray-200 dark:border-gray-700">
	<summary
		class="flex cursor-pointer items-center justify-between gap-2 px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-300"
	>
		<span>Equalizer</span>
		<span class="text-xs font-normal {toneClass[status.tone]}">{status.text}</span>
	</summary>

	<div class="space-y-3 px-3 pb-3">
		<p class="text-xs text-gray-500 dark:text-gray-400">
			The EQ belongs to the source, so every zone playing {source.name} hears it. It applies to digital
			streams only.
		</p>

		{#if !available}
			<p class="text-xs text-gray-500 dark:text-gray-400">
				CamillaDSP is not installed on this unit, so the EQ cannot run. Run the setup script to
				install it.
			</p>
		{/if}

		<fieldset disabled={!available} class="space-y-3 disabled:opacity-50">
			<label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
				<input
					type="checkbox"
					checked={draft.enabled}
					onchange={(e) => {
						draft.enabled = e.currentTarget.checked;
						scheduleSend();
					}}
				/>
				Enable EQ
			</label>

			<div class="flex flex-wrap gap-1">
				{#each presets as p (p.name)}
					<button
						type="button"
						onclick={() => applyPreset(p)}
						class="rounded border border-gray-300 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-100 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
					>
						{p.name}
					</button>
				{/each}
			</div>

			<div>
				<div class="flex items-center justify-between text-xs text-gray-600 dark:text-gray-400">
					<label for="eq-preamp-{source.id}">Preamp</label>
					<span>{draft.preamp_db.toFixed(1)} dB</span>
				</div>
				<input
					id="eq-preamp-{source.id}"
					type="range"
					min="-24"
					max="0"
					step="0.5"
					value={draft.preamp_db}
					oninput={(e) => setNumber((v) => (draft.preamp_db = v), e.currentTarget.value)}
					class="w-full"
				/>
			</div>

			{#if mayClip}
				<p class="text-xs text-amber-700 dark:text-amber-400" role="alert">
					Possible clipping: preamp {draft.preamp_db.toFixed(1)} dB plus the largest boost
					{maxBoost.toFixed(1)} dB is above 0 dB. Lower the preamp to at least -{maxBoost.toFixed(
						1
					)} dB.
				</p>
			{/if}

			{#each draft.bands as band, i (i)}
				<div class="rounded border border-gray-200 p-2 dark:border-gray-700">
					<div class="flex items-center justify-between">
						<span class="text-xs font-medium text-gray-700 dark:text-gray-300">Band {i + 1}</span>
						<button
							type="button"
							onclick={() => removeBand(i)}
							class="text-xs text-red-600 hover:underline dark:text-red-400"
							aria-label="Remove band {i + 1}">Remove</button
						>
					</div>
					<div class="mt-1 grid grid-cols-3 gap-2">
						<label class="text-xs text-gray-600 dark:text-gray-400">
							Type
							<select
								value={band.type}
								onchange={(e) => {
									band.type = e.currentTarget.value as EQBandType;
									scheduleSend();
								}}
								class={inputClass}
							>
								{#each bandTypes as t (t.value)}
									<option value={t.value}>{t.label}</option>
								{/each}
							</select>
						</label>
						<label class="text-xs text-gray-600 dark:text-gray-400">
							Freq (Hz)
							<input
								type="number"
								min="20"
								max="20000"
								step="1"
								value={band.freq}
								onchange={(e) => setNumber((v) => (band.freq = v), e.currentTarget.value)}
								class={inputClass}
							/>
						</label>
						<label class="text-xs text-gray-600 dark:text-gray-400">
							Q
							<input
								type="number"
								min="0.1"
								max="10"
								step="0.1"
								value={band.q}
								onchange={(e) => setNumber((v) => (band.q = v), e.currentTarget.value)}
								class={inputClass}
							/>
						</label>
					</div>
					<div class="mt-1">
						<div class="flex justify-between text-xs text-gray-600 dark:text-gray-400">
							<span>Gain</span>
							<span>{band.gain > 0 ? '+' : ''}{band.gain.toFixed(1)} dB</span>
						</div>
						<input
							type="range"
							min="-12"
							max="12"
							step="0.5"
							value={band.gain}
							oninput={(e) => setNumber((v) => (band.gain = v), e.currentTarget.value)}
							class="w-full"
							aria-label="Band {i + 1} gain"
						/>
					</div>
				</div>
			{/each}

			<button
				type="button"
				onclick={addBand}
				disabled={draft.bands.length >= MAX_BANDS}
				class="rounded border border-gray-300 px-2 py-1 text-xs text-gray-700 hover:bg-gray-100 disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
			>
				Add band ({draft.bands.length}/{MAX_BANDS})
			</button>
		</fieldset>

		{#if error}
			<p class="text-xs text-red-600 dark:text-red-400" role="alert">{error}</p>
		{/if}
	</div>
</details>
