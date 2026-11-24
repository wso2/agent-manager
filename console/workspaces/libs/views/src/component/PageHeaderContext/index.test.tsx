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

import { useContext, useMemo, type ReactNode } from 'react';
import { act, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PageHeaderContext, PageHeaderProvider, usePageHeader } from '.';

function HeaderSlot() {
  const { header } = useContext(PageHeaderContext);
  return <div data-testid="slot">{header}</div>;
}

// The header must keep its identity between renders (the console's React
// compiler memoizes it); a fresh element each render would loop forever.
function Page({ label }: { label: string }) {
  const header = useMemo(() => <span>{label}</span>, [label]);
  usePageHeader(header);
  return null;
}

describe('PageHeaderContext', () => {
  it('shows the header a page registers and clears it when the page unmounts', () => {
    const { rerender } = render(
      <PageHeaderProvider>
        <HeaderSlot />
        <Page label="Agents header" />
      </PageHeaderProvider>,
    );
    expect(screen.getByTestId('slot')).toHaveTextContent('Agents header');

    rerender(
      <PageHeaderProvider>
        <HeaderSlot />
      </PageHeaderProvider>,
    );
    expect(screen.getByTestId('slot')).toBeEmptyDOMElement();
  });

  it('has a no-op setter outside a provider', () => {
    let setHeader: ((header?: ReactNode) => void) | undefined;
    function Probe() {
      setHeader = useContext(PageHeaderContext).setHeader;
      return null;
    }
    render(<Probe />);
    expect(() => act(() => setHeader?.('x'))).not.toThrow();
  });
});
