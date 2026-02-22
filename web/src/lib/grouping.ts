// Utility functions for hierarchical group/zone filtering
// Based on legacy Python UI's GroupZoneFiltering.jsx

import type { Zone, Group } from './types';

/** Returns zones that belong to groups and zones that don't */
export function filterZonesByGroup(zones: Zone[], groups: Group[]): {
	grouped: Map<number, Zone[]>; // groupId -> zones in that group
	standalone: Zone[]; // zones not in any group
} {
	const grouped = new Map<number, Zone[]>();
	const zoneIdsInGroups = new Set<number>();

	// Build map of group -> zones and track which zones are in groups
	for (const group of groups) {
		const groupZones = zones.filter((z) => group.zones.includes(z.id));
		if (groupZones.length > 0) {
			grouped.set(group.id, groupZones);
			groupZones.forEach((z) => zoneIdsInGroups.add(z.id));
		}
	}

	// Find standalone zones (not in any group)
	const standalone = zones.filter((z) => !zoneIdsInGroups.has(z.id));

	return { grouped, standalone };
}

/** Returns zones for a source that belong to a specific group */
export function getGroupZones(groupId: number, sourceId: number, zones: Zone[], group: Group): Zone[] {
	return zones.filter(
		(z) => z.source_id === sourceId && group.zones.includes(z.id) && !z.disabled
	);
}

/** Returns groups that have at least one zone connected to the given source */
export function getSourceGroups(sourceId: number, zones: Zone[], groups: Group[]): Group[] {
	return groups.filter((g) => {
		return g.zones.some((zoneId) => {
			const zone = zones.find((z) => z.id === zoneId);
			return zone && zone.source_id === sourceId && !zone.disabled;
		});
	});
}
