import React from 'react';
import * as LucideIcons from 'lucide-react';
import styles from './styles.module.css';

interface DocIconProps {
  name: keyof typeof LucideIcons;
  size?: number;
  color?: string;
  className?: string;
}

/**
 * Icon component optimized for inline usage in documentation
 * Includes vertical alignment and spacing for better text integration
 * 
 * @example
 * ```mdx
 * import DocIcon from '@site/src/components/DocIcon';
 * 
 * <DocIcon name="Info" /> This is an informational message
 * <DocIcon name="AlertTriangle" color="#ff9900" /> Warning message
 * ```
 */
export default function DocIcon({ 
  name, 
  size = 18, 
  color,
  className = ''
}: DocIconProps) {
  const LucideIcon = LucideIcons[name];
  
  if (!LucideIcon) {
    console.warn(`Icon "${name}" not found in lucide-react`);
    return null;
  }
  
  return (
    <LucideIcon 
      size={size} 
      color={color}
      className={`${styles.docIcon} ${className}`}
      strokeWidth={2}
    />
  );
}
