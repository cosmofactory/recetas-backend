// Load test for the BE-6 performance target. See scripts/load/README.md.
//
// Self-contained (no local imports), so it also runs from stdin:
// `k6 run - < catalog.js`, which is how it reaches a container on the VM.

import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';

const DEFAULT_RATE = 20;
const DEFAULT_DURATION = '2m';
const TARGET_API = 'api';
const TARGET_PAGES = 'pages';
// One VU per rps is plenty at millisecond latencies; the headroom lets k6 keep
// the rate through a slow patch (up to 5s per request) instead of dropping
// iterations.
const MAX_VUS_PER_RPS = 5;

// BE-6 targets: p95 in milliseconds, per API endpoint. Every target also
// requires zero failed requests.
const API_P95_TARGETS_MS = { dishes: 100, recipe: 50 };

const LOCALES = ['en', 'es', 'ru'];
// Known keys from testdata/content.design.json (the design seed).
const DISH_KEYS = ['tortilla', 'paella', 'cocido'];
const VARIATIONS = [
	['tortilla', 'gal'],
	['paella', 'val'],
	['cocido', 'mad'],
	['croquetas', 'bas']
];

const config = readConfig();
const mix = config.target === TARGET_API ? apiMix() : pagesMix();

export const options = {
	discardResponseBodies: true,
	summaryTrendStats: ['avg', 'min', 'med', 'p(95)', 'p(99)', 'max'],
	scenarios: {
		catalog: {
			executor: 'constant-arrival-rate',
			rate: config.rate,
			timeUnit: '1s',
			duration: config.duration,
			preAllocatedVUs: config.rate,
			maxVUs: config.rate * MAX_VUS_PER_RPS
		}
	},
	thresholds: thresholds(mix, config.target)
};

export default function () {
	const next = mix[exec.scenario.iterationInTest % mix.length];
	const response = http.get(config.baseUrl + next.path, {
		tags: { name: next.name, endpoint: next.endpoint }
	});
	check(response, { 'status is 200': (r) => r.status === 200 });
}

function readConfig() {
	const baseUrl = (__ENV.BASE_URL || '').replace(/\/+$/, '');
	if (!baseUrl) {
		throw new Error('BASE_URL is required, e.g. BASE_URL=http://localhost:8080');
	}
	const rate = Number(__ENV.RATE || DEFAULT_RATE);
	if (!Number.isInteger(rate) || rate < 1) {
		throw new Error(`RATE must be a positive integer, got "${__ENV.RATE}"`);
	}
	const target = __ENV.TARGET || TARGET_API;
	if (target !== TARGET_API && target !== TARGET_PAGES) {
		throw new Error(`TARGET must be ${TARGET_API} or ${TARGET_PAGES}, got "${target}"`);
	}
	return { baseUrl, rate, target, duration: __ENV.DURATION || DEFAULT_DURATION };
}

// One request per iteration, round-robin over this list. The dish list, the
// headline target, gets twice the share of the region list.
function apiMix() {
	return LOCALES.flatMap((lang) => [
		request('regions', 'GET /v1/regions', `/v1/regions?lang=${lang}`),
		request('dishes', 'GET /v1/dishes', `/v1/dishes?lang=${lang}`),
		request('dishes', 'GET /v1/dishes', `/v1/dishes?lang=${lang}`),
		...DISH_KEYS.map((dish) =>
			request('dish', 'GET /v1/dishes/{dishKey}', `/v1/dishes/${dish}?lang=${lang}`)
		),
		...VARIATIONS.map(([dish, region]) =>
			request(
				'recipe',
				'GET /v1/dishes/{dishKey}/variations/{regionKey}/recipe',
				`/v1/dishes/${dish}/variations/${region}/recipe?lang=${lang}`
			)
		)
	]);
}

// The public pages whose server-side loads call the API (recetas-frontend
// src/routes/[lang=lang]): the home page reads the cached lists only, the dish
// page adds GET /v1/dishes/{dishKey}, the recipe page adds the recipe call.
function pagesMix() {
	return LOCALES.flatMap((lang) => [
		request('home-page', 'GET /:lang/', `/${lang}/`),
		...DISH_KEYS.map((dish) =>
			request('dish-page', 'GET /:lang/dishes/:dishKey', `/${lang}/dishes/${dish}`)
		),
		...VARIATIONS.map(([dish, region]) =>
			request(
				'recipe-page',
				'GET /:lang/dishes/:dishKey/:regionKey',
				`/${lang}/dishes/${dish}/${region}`
			)
		)
	]);
}

function request(endpoint, name, path) {
	return { endpoint, name, path };
}

// An empty list declares the tagged sub-metric without a limit, so the summary
// shows p50/p95/p99 and the error rate for every endpoint, not only the
// targeted ones. A dropped iteration is a request the rate called for but
// never sent, so it fails the run too.
function thresholds(requests, target) {
	const result = {
		http_req_failed: ['rate==0'],
		checks: ['rate==1'],
		dropped_iterations: ['count==0']
	};
	for (const endpoint of new Set(requests.map((r) => r.endpoint))) {
		const p95 = API_P95_TARGETS_MS[endpoint];
		const limit = target === TARGET_API && p95 ? [`p(95)<${p95}`] : [];
		result[`http_req_duration{endpoint:${endpoint}}`] = limit;
		result[`http_req_failed{endpoint:${endpoint}}`] = [];
	}
	return result;
}
