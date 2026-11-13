// Contract test (H3): the client's API types are generated from the pinned gateway
// schema and must not drift from it. Typed fixtures in testing.ts cover the other
// direction: a schema change that breaks them fails `npm run check`.
import { readFile } from 'node:fs/promises';
import { expect, it } from 'vitest';
import openapiTS, { astToString } from 'openapi-typescript';

const root = new URL('../../', import.meta.url);

it('generated API types match the pinned gateway schema', async () => {
	const schema = new URL('schema/gateway.openapi.json', root);
	const committed = await readFile(new URL('src/lib/gateway-schema.d.ts', root), 'utf8');

	const generated = astToString(await openapiTS(schema));

	// The CLI adds a header comment to the same output.
	expect(committed.endsWith(generated), 'Run `npm run gen:api` and commit the result.').toBe(true);
});
