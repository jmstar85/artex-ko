"use client";

import * as React from "react";

import { format, subDays } from "date-fns";
import { useLocale, useTranslations } from "next-intl";
import type { DateRange } from "react-day-picker";
import { enUS, ko, zhCN } from "react-day-picker/locale";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

interface DateRangePickerProps {
  value?: DateRange;
  onChange?: (value: DateRange | undefined) => void;
}

const DATE_LOCALES = { ko, en: enUS, zh: zhCN } as const;

export function DateRangePicker({ value, onChange }: DateRangePickerProps) {
  const t = useTranslations("ui.dateRangePicker");
  const uiLocale = useLocale();
  const dateLocale = DATE_LOCALES[uiLocale as keyof typeof DATE_LOCALES] ?? enUS;
  const [open, setOpen] = React.useState(false);
  const [internalDateRange, setInternalDateRange] = React.useState<DateRange | undefined>(() => {
    const to = new Date();
    const from = subDays(to, 29);
    return { from, to };
  });
  const dateRange = value ?? internalDateRange;
  let dateRangeLabel = t("selectDate");

  if (dateRange?.from) {
    dateRangeLabel = format(dateRange.from, "PP", { locale: dateLocale });
  }

  if (dateRange?.from && dateRange.to) {
    dateRangeLabel = `${format(dateRange.from, "PP", { locale: dateLocale })} - ${format(dateRange.to, "PP", { locale: dateLocale })}`;
  }

  const handleDateChange = (nextValue: DateRange | undefined) => {
    if (!value) {
      setInternalDateRange(nextValue);
    }
    onChange?.(nextValue);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" id="date" className="font-normal">
          {dateRangeLabel}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto overflow-hidden p-0" align="end">
        <Calendar
          mode="range"
          defaultMonth={dateRange?.from}
          selected={dateRange}
          onSelect={handleDateChange}
          numberOfMonths={2}
        />
      </PopoverContent>
    </Popover>
  );
}
