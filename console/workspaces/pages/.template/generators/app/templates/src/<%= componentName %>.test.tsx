/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { render, screen } from '@testing-library/react';
import { 
  <%= componentName %>Component,
  <%= componentName %>Project,
  <%= componentName %>Organization,
} from './index';

describe('<%= componentName %>Component', () => {
  it('renders without crashing', () => {
    render(<<%= componentName %>Component />);
    expect(screen.getByText('<%= title %> - Component Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Title';
    render(<<%= componentName %>Component title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Description';
    render(<<%= componentName %>Component description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays component level indicator', () => {
    render(<<%= componentName %>Component />);
    expect(screen.getByText('Component Level View')).toBeInTheDocument();
  });
});

describe('<%= componentName %>Project', () => {
  it('renders without crashing', () => {
    render(<<%= componentName %>Project />);
    expect(screen.getByText('<%= title %> - Project Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Project Title';
    render(<<%= componentName %>Project title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Project Description';
    render(<<%= componentName %>Project description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays project level indicator', () => {
    render(<<%= componentName %>Project />);
    expect(screen.getByText('Project Level View')).toBeInTheDocument();
  });
});

describe('<%= componentName %>Organization', () => {
  it('renders without crashing', () => {
    render(<<%= componentName %>Organization />);
    expect(screen.getByText('<%= title %> - Organization Level')).toBeInTheDocument();
  });

  it('renders with custom title', () => {
    const customTitle = 'Custom Organization Title';
    render(<<%= componentName %>Organization title={customTitle} />);
    expect(screen.getByText(customTitle)).toBeInTheDocument();
  });

  it('renders with custom description', () => {
    const customDescription = 'Custom Organization Description';
    render(<<%= componentName %>Organization description={customDescription} />);
    expect(screen.getByText(customDescription)).toBeInTheDocument();
  });

  it('displays organization level indicator', () => {
    render(<<%= componentName %>Organization />);
    expect(screen.getByText('Organization Level View')).toBeInTheDocument();
  });
});
