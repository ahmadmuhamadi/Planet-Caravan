// Wording for the API warnings (F9). Text from the Help page design ("Notices").
import type { Warning } from './api';

export type WarningText = { label: string; help: string };

const TEXT: Record<Warning, WarningText> = {
	low_alignment_quality: {
		label: 'Low alignment quality',
		help: 'This frame did not line up with the others as well as required. An apparent shift may not be real.'
	},
	not_comparable: {
		label: 'Not comparable',
		help: 'The two measurements were taken at different wavelengths, so their brightness cannot be compared.'
	},
	catalog_check_incomplete: {
		label: 'Catalogue check incomplete',
		help: 'A catalogue service did not answer. Unmatched sources are not marked as candidates until it does.'
	},
	saturated: {
		label: 'Saturated',
		help: 'The object is too bright for the detector here. Brightness and position are not measured.'
	},
	upper_limit: {
		label: 'Upper limit',
		help: 'The object was not detected in this epoch. The value is the faintest it could have been seen.'
	}
};

/** Text for a warning code. Codes outside the contract's vocabulary are shown as they are. */
export function warningText(code: string): WarningText {
	return TEXT[code as Warning] ?? { label: code.replaceAll('_', ' '), help: '' };
}
