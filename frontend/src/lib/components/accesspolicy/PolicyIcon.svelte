<script lang="ts">
	import { getPolicyIconComponent, resolvePolicyIcon } from '$lib/utils/policy-icon';

	interface Props {
		label: string;
		policyName?: string;
		isHydraRoute?: boolean;
		size?: number;
		strokeWidth?: number;
	}

	let {
		label,
		policyName = '',
		isHydraRoute = false,
		size = 28,
		strokeWidth = 1.5,
	}: Props = $props();

	const iconId = $derived(resolvePolicyIcon(label, { policyName, isHydraRoute }));
	const Icon = $derived(getPolicyIconComponent(iconId));
</script>

<Icon {size} {strokeWidth} />
