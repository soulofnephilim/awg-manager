import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, fireEvent, screen } from '@testing-library/svelte';
import PresetApplyModal from './PresetApplyModal.svelte';
import type { SingboxRouterPreset } from '$lib/types';

// jsdom has no ResizeObserver — the shared Dropdown uses one for portal
// placement. A noop stub is enough for the tests below.
beforeAll(() => {
	if (typeof globalThis.ResizeObserver === 'undefined') {
		globalThis.ResizeObserver = class {
			observe() {}
			unobserve() {}
			disconnect() {}
		} as unknown as typeof ResizeObserver;
	}
});

const tunnelPreset = (id: string, name: string): SingboxRouterPreset => ({
	id, name, category: 'social', iconSlug: id,
	ruleSets: [{ tag: `geosite-${id}`, url: `https://example/${id}.srs` }],
	rules: [{ ruleSetRef: `geosite-${id}`, actionTarget: 'tunnel' }],
});
const rejectPreset = (id: string, name: string): SingboxRouterPreset => ({
	id, name, category: 'block', iconSlug: id,
	ruleSets: [{ tag: `geosite-${id}`, url: `https://example/${id}.srs` }],
	rules: [{ ruleSetRef: `geosite-${id}`, actionTarget: 'reject' }],
});

const dnsServers = [
	{ tag: 'cloudflare', type: 'udp', server: '1.1.1.1' },
	{ tag: 'google', type: 'udp', server: '8.8.8.8' },
];
const outboundOptions = [
	{ group: 'AWG', items: [{ value: 'awg-1', label: 'YAawg' }, { value: 'awg-2', label: 'NuxtAWG' }] },
];

describe('PresetApplyModal', () => {
	it('renders single-preset title', () => {
		render(PresetApplyModal, {
			props: {
				presets: [tunnelPreset('telegram', 'Telegram')],
				outboundOptions, dnsServers,
				onClose: vi.fn(), onApply: vi.fn(),
			},
		});
		expect(screen.getByText(/Применить пресет: Telegram/)).toBeTruthy();
	});

	it('renders batch title with count', () => {
		render(PresetApplyModal, {
			props: {
				presets: [tunnelPreset('telegram', 'Telegram'), tunnelPreset('discord', 'Discord')],
				outboundOptions, dnsServers,
				onClose: vi.fn(), onApply: vi.fn(),
			},
		});
		expect(screen.getByText(/Применить 2 пресета/i)).toBeTruthy();
	});

	it('hides DNS section when all presets are reject', () => {
		render(PresetApplyModal, {
			props: {
				presets: [rejectPreset('ads', 'Ads block')],
				outboundOptions, dnsServers,
				onClose: vi.fn(), onApply: vi.fn(),
			},
		});
		expect(screen.queryByText(/DNS-правило/i)).toBeNull();
	});

	it('shows DNS section when at least one preset is tunnel', () => {
		render(PresetApplyModal, {
			props: {
				presets: [tunnelPreset('telegram', 'Telegram'), rejectPreset('ads', 'Ads')],
				outboundOptions, dnsServers,
				onClose: vi.fn(), onApply: vi.fn(),
			},
		});
		expect(screen.getByText(/DNS-правило/i)).toBeTruthy();
	});

	it('Apply disabled until tunnel chosen', () => {
		render(PresetApplyModal, {
			props: {
				presets: [tunnelPreset('telegram', 'Telegram')],
				outboundOptions, dnsServers,
				onClose: vi.fn(), onApply: vi.fn(),
			},
		});
		const apply = screen.getByRole('button', { name: /применить/i });
		// NOTE: jest-dom matchers (toHaveAttribute) aren't wired in this repo;
		// using hasAttribute (matches the existing PresetsBulkBar.test.ts style).
		expect(apply.hasAttribute('disabled')).toBe(true);
	});

	it('Apply payload has presetIds + dns flags', async () => {
		const onApply = vi.fn().mockResolvedValue(undefined);
		render(PresetApplyModal, {
			props: {
				presets: [tunnelPreset('telegram', 'Telegram')],
				outboundOptions, dnsServers,
				onClose: vi.fn(), onApply,
			},
		});
		// The shared Dropdown component is a button-triggered listbox (not a
		// native <select> / role="combobox"), so we click the trigger and
		// click the option instead of firing a `change` event. The trigger's
		// accessible name comes from the wrapping <label> div text.
		const outboundTrigger = screen.getByRole('button', { name: /Направить трафик/i });
		await fireEvent.click(outboundTrigger);
		const awg1Option = await screen.findByRole('option', { name: /YAawg/ });
		await fireEvent.click(awg1Option);

		// Check DNS checkbox
		const dnsCheckbox = screen.getByRole('checkbox', { name: /DNS-правило/i });
		await fireEvent.click(dnsCheckbox);
		// Open DNS dropdown and pick cloudflare. The DNS field is a <div>, not
		// a <label>, so the trigger's accessible name is the visible placeholder.
		const dnsTrigger = await screen.findByRole('button', { name: /выберите DNS-сервер/i });
		await fireEvent.click(dnsTrigger);
		const cfOption = await screen.findByRole('option', { name: /cloudflare/i });
		await fireEvent.click(cfOption);

		const apply = screen.getByRole('button', { name: /применить/i });
		await fireEvent.click(apply);
		expect(onApply).toHaveBeenCalledWith({
			presetIds: ['telegram'],
			outboundTag: 'awg-1',
			createDnsRule: true,
			dnsServerTag: 'cloudflare',
		});
	});

	it('triggers onCreateDnsServer from "Create new" button', async () => {
		const onCreate = vi.fn();
		render(PresetApplyModal, {
			props: {
				presets: [tunnelPreset('telegram', 'Telegram')],
				outboundOptions, dnsServers: [],
				onClose: vi.fn(), onApply: vi.fn(), onCreateDnsServer: onCreate,
			},
		});
		const dnsCheckbox = screen.getByRole('checkbox', { name: /DNS-правило/i });
		await fireEvent.click(dnsCheckbox);
		const createBtn = screen.getByRole('button', { name: /создать DNS-сервер/i });
		await fireEvent.click(createBtn);
		expect(onCreate).toHaveBeenCalledOnce();
	});
});
