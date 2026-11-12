import { defineEnvVars } from '@sveltejs/kit/env';

export const variables = defineEnvVars({
	PUBLIC_GATEWAY_URL: {
		public: true,
		static: true,
		description:
			'Public gateway base URL, inlined into the static build (H10: public values only).',
		schema: (value) => value || 'http://localhost:8080'
	}
});
