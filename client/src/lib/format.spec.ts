import { describe, expect, it } from 'vitest';
import { formatAngle, formatBand, formatDec, formatFov, formatRA, scaleBar } from './format';

describe('formatRA', () => {
	it.each([
		[269.452, '17h 57m 48.5s'],
		[0, '00h 00m 00.0s'],
		[15, '01h 00m 00.0s'],
		// 23h 59m 59.96s rounds up to the next day.
		[359.99983, '00h 00m 00.0s'],
		[10.68470833, '00h 42m 44.3s']
	])('%f → %s', (deg, text) => expect(formatRA(deg)).toBe(text));
});

describe('formatDec', () => {
	it.each([
		[4.693, '+04° 41′ 35″'],
		[-5.5, '−05° 30′ 00″'],
		[90, '+90° 00′ 00″'],
		[-0.0001, '−00° 00′ 00″'],
		[41.26875, '+41° 16′ 08″']
	])('%f → %s', (deg, text) => expect(formatDec(deg)).toBe(text));
});

describe('small formats', () => {
	it('formats bands, angles and fields', () => {
		expect(formatBand(1.1, 1.65)).toBe('1.10–1.65');
		expect(formatAngle(0.4)).toBe('0.4″');
		expect(formatAngle(30)).toBe('30″');
		expect(formatAngle(120)).toBe('2′');
		expect(formatAngle(90)).toBe('1.5′');
		expect(formatFov(3)).toBe('3′ × 3′');
	});
});

describe('scaleBar', () => {
	it('picks the longest round length up to a quarter of the field', () => {
		expect(scaleBar(180)).toEqual({ arcsec: 30, fraction: 30 / 180 }); // 3′ field
		expect(scaleBar(1800)).toEqual({ arcsec: 300, fraction: 300 / 1800 }); // 30′ field
		expect(scaleBar(2)).toEqual({ arcsec: 1, fraction: 0.5 });
	});
});
