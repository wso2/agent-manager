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

import {
  alpha,
  Box,
  Button,
  ButtonBase,
  Chip,
  Divider,
  IconButton,
  Menu,
  MenuItem,
  MenuList,
  TextField,
  Typography,
  useTheme,
} from '@wso2/oxygen-ui';
import {
  Plus as AddOutlined,
  X as CloseOutlined,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Search as SearchOutlined,
} from '@wso2/oxygen-ui-icons-react';
import { useState, useMemo } from 'react';
import { FadeIn } from '../../FadeIn';

export interface Option {
  label: string;
  typeLabel?: string;
  id: string;
}
export interface TopSelecterProps {
  label: string;
  onChange: (value: string) => void;
  options: Option[];
  selectedId?: string;
  disableClose: boolean;
  onClose?: () => void;
  onClick: () => void;
  onCreate?: () => void;
}

export function TopSelecter(props: TopSelecterProps) {
  const {
    label,
    onChange,
    options,
    selectedId,
    disableClose,
    onClose,
    onClick,
    onCreate,
  } = props;
  const theme = useTheme();
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
    setSearchQuery('');
  };

  const handleSelectOption = (optionId: string) => {
    onChange(optionId);
    handleClose();
  };

  const filteredOptions = useMemo(() => {
    if (!searchQuery.trim()) {
      return options;
    }
    return options.filter((option) =>
      option.label.toLowerCase().includes(searchQuery.toLowerCase())
    );
  }, [options, searchQuery]);

  const selectedOption = options.find((opt) => opt.id === selectedId);

  const open = Boolean(anchorEl);
  const id = open ? 'top-selector-popover' : undefined;

  if (options.length === 0) {
    return null;
  }
  return (
    <>
      {selectedId ? (
        <FadeIn>
          <ButtonBase
            onClick={onClick}
            sx={{
              gap: 4,
              padding: 0.125,
              pl: 1,
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'start',
              borderRadius: 0.5,
              color: theme.palette.text.primary,
              transition: 'all 0.3s ease',
              border: `1px solid ${alpha(theme.palette.text.primary, 0.1)}`,
              '&:hover': {
                backgroundColor: 'background.paper',
                border: `1px solid ${theme.palette.primary.light}`,
                cursor: 'pointer',
              },
            }}
          >
            <Box
              display="flex"
              alignItems="flex-start"
              flexDirection="column"
              padding={0.5}
              gap={0.25}
            >
              <Typography variant="caption" color="textSecondary">
                {label}
              </Typography>
              <Box display="flex" alignItems="center" gap={0.5}>
                <Typography
                  className="selector-name-button"
                  variant="body1"
                  color="textPrimary"
                >
                  {selectedOption?.label || 'Select an option'}
                </Typography>
                <IconButton
                  size="small"
                  onClick={handleClick}
                  sx={{
                    borderRadius: 0.5,
                    padding: 0,
                  }}
                >
                  {open ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                </IconButton>
              </Box>
            </Box>
            {onClose ? (
              <IconButton
                size="small"
                disabled={disableClose}
                onClick={(e) => {
                  e.stopPropagation();
                  onClose();
                }}
              >
                <CloseOutlined size={16} />
              </IconButton>
            ) : (
              <Box width={1.75} />
            )}
          </ButtonBase>
        </FadeIn>
      ) : (
        <FadeIn>
          <ButtonBase
            onClick={handleClick}
            sx={{
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              padding: 1,
              borderRadius: 0.5,
              transition: 'all 0.3s ease',
              color: 'text.primary',
              border: `1px solid ${alpha(theme.palette.text.primary, 0.1)}`,
              '&:hover': {
                backgroundColor: 'background.paper',
                border: `1px solid ${theme.palette.primary.light}`,
                cursor: 'pointer',
              },
            }}
          >
            {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          </ButtonBase>
        </FadeIn>
      )}
      <Menu
        id={id}
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
      >
        <Box px={1} display="flex" flexDirection="column" gap={0.5}>
          <TextField
            fullWidth
            size="small"
            placeholder="Search..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            variant="outlined"
            slotProps={{
              input: {
                endAdornment: <SearchOutlined size={16} />,
              },
              inputLabel: {
                shrink: true,
                sx: {
                  color: theme.palette.text.secondary,
                },
              },
            }}
          />
          {onCreate && (
            <Box>
              <Button
                sx={{ pl: 0.5 }}
                variant="text"
                startIcon={<AddOutlined size={16} />}
                size="small"
                onClick={() => {
                  onCreate();
                  handleClose();
                }}
              >
                Add {label}
              </Button>
            </Box>
          )}
          <Divider />
          <MenuList >
            {filteredOptions.length > 0 ? (
              filteredOptions.map((option) => (
                <MenuItem
                  key={option.id}
                  onClick={() => handleSelectOption(option.id)}
                  selected={option.id === selectedId}
                >
                  <Typography>
                    {option.label}
                    &nbsp;
                    {option.typeLabel && (
                      <Chip
                        size="small"
                        variant="outlined"
                        label={option.typeLabel}
                        color="default"
                      />
                    )}
                  </Typography>
                </MenuItem>
              ))
            ) : (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{
                  padding: 2,
                  textAlign: 'center',
                }}
              >
                No results found
              </Typography>
            )}
          </MenuList>
        </Box>
      </Menu>
    </>
  );
}
