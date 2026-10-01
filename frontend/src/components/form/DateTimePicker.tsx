import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CloseCircleFilled } from '@ant-design/icons';
import { DatePicker, theme } from 'antd';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import { PersianDateTimePicker } from 'persian-calendar-suite';

import { useDatepicker } from '@/hooks/useDatepicker';
import { useTheme } from '@/hooks/useTheme';
import './DateTimePicker.css';

interface DateTimePickerProps {
  value: Dayjs | null;
  onChange: (next: Dayjs | null) => void;
  showTime?: boolean;
  format?: string;
  placeholder?: string;
  disabled?: boolean;
  allowClear?: boolean;
  maxDate?: Dayjs;
}

export default function DateTimePicker({
  value,
  onChange,
  showTime = true,
  format = 'YYYY-MM-DD HH:mm:ss',
  placeholder = '',
  disabled = false,
  allowClear = true,
  maxDate,
}: DateTimePickerProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const { isDark, isUltra } = useTheme();
  const jalaliRef = useRef<HTMLDivElement>(null);
  // Bumped on clear: persian-calendar-suite reads `value` only on mount, so
  // remounting via key is the only way to reflect an externally cleared value.
  const [clearNonce, setClearNonce] = useState(0);
  // Mounted without a value, persian-calendar-suite seeds today and emits it —
  // which would instantly undo a clear. Armed across every (re)mount.
  const suppressMountEmit = useRef(true);

  useEffect(() => {
    suppressMountEmit.current = false;
    return () => {
      suppressMountEmit.current = true;
    };
  }, [clearNonce]);

  const { token } = theme.useToken();
  // colorLink is the theme's AA-safe accent: white text on it in light mode, dark
  // text in dark mode, where the bright coral would not carry white text.
  const persianTheme = useMemo(
    () => ({
      primaryColor: token.colorLink,
      backgroundColor: token.colorBgElevated,
      borderColor: token.colorBorder,
      hoverColor: token.colorPrimaryBg,
      selectedTextColor: isDark ? token.colorBgContainer : token.colorTextLightSolid,
      textColor: token.colorText,
    }),
    [token, isDark],
  );

  const commitChange = (next: Dayjs | null) => {
    if (next && maxDate && next.isAfter(maxDate)) {
      if (datepicker === 'jalalian') setClearNonce((n) => n + 1);
      return;
    }
    onChange(next);
  };

  // The library hardcodes a Persian placeholder and exposes no working prop to
  // override it, so clear it (or apply the caller's) on the input directly so
  // the empty field shows no leftover Persian text. No dep array: re-apply
  // after every render (incl. clear-remounts).
  useEffect(() => {
    if (datepicker !== 'jalalian') return;
    const input = jalaliRef.current?.querySelector('input');
    if (input) input.placeholder = placeholder;
  });

  if (datepicker === 'jalalian') {
    return (
      <div
        ref={jalaliRef}
        className={`jdp-wrap${isDark ? ' jdp-dark' : ''}${isUltra ? ' jdp-ultra' : ''}${disabled ? ' jdp-disabled' : ''}${value ? '' : ' jdp-empty'}`}
      >
        <PersianDateTimePicker
          key={clearNonce}
          value={value ? value.valueOf() : null}
          onChange={(next: number | string | null) => {
            if (suppressMountEmit.current) return;
            if (next == null || next === '') {
              commitChange(null);
              return;
            }
            const ms = typeof next === 'number' ? next : Number(next);
            if (Number.isFinite(ms)) commitChange(dayjs(ms));
          }}
          showTime={showTime}
          outputFormat="timestamp"
          maxDate={maxDate?.toDate()}
          persianNumbers
          rtlCalendar
          theme={persianTheme}
        />
        {value && allowClear && !disabled && (
          <button
            type="button"
            className="jdp-clear"
            aria-label={t('clear')}
            onMouseDown={(e) => e.preventDefault()}
            onClick={(e) => {
              e.stopPropagation();
              commitChange(null);
              setClearNonce((n) => n + 1);
            }}
          >
            <CloseCircleFilled />
          </button>
        )}
      </div>
    );
  }

  return (
    <DatePicker
      value={value}
      onChange={(next) => commitChange(next || null)}
      onCalendarChange={(next) => commitChange((Array.isArray(next) ? next[0] : next) || null)}
      showTime={showTime ? { format: 'HH:mm:ss' } : false}
      needConfirm={false}
      format={format}
      placeholder={placeholder}
      disabled={disabled}
      allowClear={allowClear}
      maxDate={maxDate}
      style={{ width: '100%' }}
    />
  );
}
