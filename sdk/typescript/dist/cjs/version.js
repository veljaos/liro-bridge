"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SDK_VERSION = void 0;
/**
 * This SDK's own version, as `package.json` states it.
 *
 * It is written out rather than read from `package.json` at run time,
 * because reading it would mean this package resolving a path relative
 * to itself — which is one thing in an ESM build, another in a
 * CommonJS one, and a third inside a bundler that has flattened both.
 * The check that keeps it honest is a test that reads `package.json`
 * and compares.
 */
exports.SDK_VERSION = '1.0.0';
