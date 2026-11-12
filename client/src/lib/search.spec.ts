import { describe, expect, it } from 'vitest';
import { parseQuery } from './search';

describe('parseQuery', () => {
	it.each([
		['269.452 4.693', 269.452, 4.693],
		['269.452, 4.693', 269.452, 4.693],
		['  10,-5.5 ', 10, -5.5],
		['0 +90', 0, 90],
		['.5 -.25', 0.5, -0.25]
	])('reads %j as coordinates', (input, ra, dec) => {
		expect(parseQuery(input)).toEqual({ kind: 'coords', ra, dec });
	});

	it.each(["Barnard's Star", 'M31', 'WISE 0855', 'NGC 1976', '12 abc'])(
		'reads %j as a name',
		(input) => {
			expect(parseQuery(input)).toEqual({ kind: 'name', name: input });
		}
	);

	it.each(['', '   ', '360 0', '-1 0', '10 91', '10 -90.5'])('rejects %j', (input) => {
		expect(parseQuery(input).kind).toBe('invalid');
	});
});
