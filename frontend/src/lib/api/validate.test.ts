import { describe, it, expect } from 'vitest';
import { findResponseSchema, validateApiResponse, ApiValidationError } from './validate';
import { RESPONSE_SCHEMAS } from './schemas.gen';

describe('findResponseSchema', () => {
	it('находит статический эндпоинт и игнорирует query-строку', () => {
		expect(findResponseSchema('GET', '/tunnels/list')).toBeDefined();
		expect(findResponseSchema('GET', '/tunnels/get?id=abc')).toBe(
			findResponseSchema('GET', '/tunnels/get'),
		);
	});

	it('различает методы', () => {
		// /tunnels/update — только POST в спеке
		expect(findResponseSchema('POST', '/tunnels/update')).toBeDefined();
		expect(findResponseSchema('GET', '/tunnels/update')).toBeUndefined();
	});

	it('матчит шаблонные пути по сегментам', () => {
		expect(findResponseSchema('GET', '/managed-servers/srv-1')).toBeDefined();
		expect(findResponseSchema('POST', '/managed-servers/srv-1/peers')).toBeDefined();
		expect(findResponseSchema('GET', '/managed-servers/srv-1/peers/PUBKEY==/conf')).toBeDefined();
		// лишний сегмент — не совпадение
		expect(findResponseSchema('GET', '/managed-servers/srv-1/peers/extra/level/deep')).toBeUndefined();
	});

	it('неизвестный эндпоинт — undefined (без проверки)', () => {
		expect(findResponseSchema('GET', '/no/such/endpoint')).toBeUndefined();
		expect(() => validateApiResponse('GET', '/no/such/endpoint', { total: 'garbage' })).not.toThrow();
	});
});

describe('validateApiResponse', () => {
	it('валидный конверт проходит', () => {
		expect(() =>
			validateApiResponse('GET', '/tunnels/list', {
				success: true,
				data: [{ id: 't1', name: 'Home', enabled: true }],
			}),
		).not.toThrow();
	});

	it('пропущенные и null-поля валидны (Go omitempty / nil-слайсы)', () => {
		expect(() => validateApiResponse('GET', '/tunnels/list', { success: true })).not.toThrow();
		expect(() =>
			validateApiResponse('GET', '/tunnels/list', { success: true, data: null }),
		).not.toThrow();
	});

	it('неизвестные поля валидны (эволюция бэка не ломает старый фронт)', () => {
		expect(() =>
			validateApiResponse('GET', '/tunnels/list', {
				success: true,
				data: [{ id: 't1', brandNewField: { anything: 1 } }],
			}),
		).not.toThrow();
	});

	it('присутствующее значение неверного типа — ApiValidationError с путём', () => {
		let caught: unknown;
		try {
			validateApiResponse('GET', '/tunnels/list', {
				success: true,
				data: [{ id: 42 }], // id должен быть string
			});
		} catch (e) {
			caught = e;
		}
		expect(caught).toBeInstanceOf(ApiValidationError);
		const err = caught as ApiValidationError;
		expect(err.endpoint).toBe('/tunnels/list');
		expect(err.issues.join(' ')).toContain('id');
		expect(err.message).toContain('/tunnels/list');
	});

	it('data неверной формы (объект вместо массива) — ошибка', () => {
		expect(() =>
			validateApiResponse('GET', '/tunnels/list', { success: true, data: { not: 'array' } }),
		).toThrow(ApiValidationError);
	});
});

describe('schemas.gen.ts инварианты', () => {
	it('покрытие спеки существенное', () => {
		expect(Object.keys(RESPONSE_SCHEMAS).length).toBeGreaterThan(250);
	});

	it('каждая схема принимает минимальный валидный конверт', () => {
		// Все поля optional+nullable by design, поэтому {} и {success:true}
		// обязаны проходить любую сгенерированную схему. Ловит регрессию
		// генератора, случайно сделавшего поле обязательным.
		for (const [key, _schema] of Object.entries(RESPONSE_SCHEMAS)) {
			const [method, path] = key.split(' ', 2);
			expect(
				() => validateApiResponse(method, path.replaceAll(/\{[^}]+\}/g, 'x'), { success: true }),
				`схема ${key} отвергла минимальный конверт`,
			).not.toThrow();
		}
	});
});
