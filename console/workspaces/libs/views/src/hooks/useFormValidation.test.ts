/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import { useFormValidation } from './useFormValidation';

const schema = z
  .object({
    name: z.string().min(1, 'Name is required'),
    password: z.string().min(8, 'Too short'),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, {
    message: 'Passwords must match',
    path: ['confirm'],
  });

const plainSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  age: z.number().min(18, 'Must be an adult'),
});

type Form = z.infer<typeof schema>;

describe('useFormValidation', () => {
  it('validateField checks a single field against its own schema', () => {
    const { result } = renderHook(() => useFormValidation(plainSchema));
    expect(result.current.validateField('name', '')).toBe('Name is required');
    expect(result.current.validateField('name', 'ok')).toBeUndefined();
    expect(result.current.validateField('age', 10)).toBe('Must be an adult');
  });

  it('validateField ignores fields the schema has no shape for', () => {
    const { result } = renderHook(() => useFormValidation(plainSchema));
    expect(
      result.current.validateField('unknown' as 'name', 'anything'),
    ).toBeUndefined();
  });

  it('validateField with full data catches cross-field refinements', () => {
    const { result } = renderHook(() => useFormValidation<Form>(schema));
    const data = { name: 'x', password: 'longenough', confirm: 'different' };
    expect(result.current.validateField('confirm', data.confirm, data)).toBe(
      'Passwords must match',
    );
    // A field with no issue of its own reports nothing even though the form fails.
    expect(result.current.validateField('name', data.name, data)).toBeUndefined();
    const valid = { ...data, confirm: 'longenough' };
    expect(result.current.validateField('confirm', valid.confirm, valid)).toBeUndefined();
  });

  it('validateForm collects one message per failing field and clears them when valid', () => {
    const { result } = renderHook(() => useFormValidation(plainSchema));

    let ok = true;
    act(() => {
      ok = result.current.validateForm({ name: '', age: 3 });
    });
    expect(ok).toBe(false);
    expect(result.current.errors).toEqual({
      name: 'Name is required',
      age: 'Must be an adult',
    });

    act(() => {
      ok = result.current.validateForm({ name: 'Ann', age: 30 });
    });
    expect(ok).toBe(true);
    expect(result.current.errors).toEqual({});
  });

  it('setFieldError, clearFieldError and clearErrors edit the error map', () => {
    const { result } = renderHook(() => useFormValidation(plainSchema));

    act(() => result.current.setFieldError('name', 'Taken'));
    act(() => result.current.setFieldError('age', 'Bad'));
    expect(result.current.errors).toEqual({ name: 'Taken', age: 'Bad' });

    act(() => result.current.setFieldError('age', undefined));
    expect(result.current.errors).toEqual({ name: 'Taken' });

    act(() => result.current.setFieldError('age', 'Bad'));
    act(() => result.current.clearFieldError('name'));
    expect(result.current.errors).toEqual({ age: 'Bad' });

    act(() => result.current.clearErrors());
    expect(result.current.errors).toEqual({});
  });
});
