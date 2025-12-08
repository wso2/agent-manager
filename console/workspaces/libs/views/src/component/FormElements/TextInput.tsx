import {
  FormControl,
  FormLabel,
  TextField,
  TextFieldProps,
  IconButton,
  Tooltip,
  InputAdornment,
} from '@wso2/oxygen-ui';
import { Copy as ContentCopy } from '@wso2/oxygen-ui-icons-react';
import { useState } from 'react';

export interface TextInputProps extends Omit<TextFieldProps, 'variant'> {
  label?: string;
  copyable?: boolean;
  copyTooltipText?: string;
}

export const TextInput = ({ 
  label, 
  copyable = false,
  copyTooltipText,
  value,
  slotProps,
  ...props 
}: TextInputProps) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    if (typeof value === 'string' && value) {
      try {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } catch {
        // Failed to copy - silently fail
      }
    }
  };

  const getCopyTooltipText = () => {
    if (copyTooltipText) {
      return copied ? 'Copied!' : copyTooltipText;
    }
    return copied ? 'Copied!' : 'Copy';
  };

  const endAdornment = copyable && typeof value === 'string' && value ? (
    <InputAdornment position="end">
      <Tooltip title={getCopyTooltipText()}>
        <IconButton
          onClick={handleCopy}
          edge="end"
          size="small"
        >
          <ContentCopy size={16} />
        </IconButton>
      </Tooltip>
    </InputAdornment>
  ) : undefined;

  const mergedSlotProps = {
    ...slotProps,
    input: {
      ...slotProps?.input,
      ...(endAdornment && { endAdornment }),
    },
  };

  return (
    <FormControl fullWidth>
      {label && <FormLabel htmlFor={label}>{label}</FormLabel>}
      <TextField
        id={label}
        sx={{
          minWidth: 100,
        }}
        variant="outlined"
        value={value}
        slotProps={mergedSlotProps}
        {...props}
      />
    </FormControl>
  );
};
