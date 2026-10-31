package org.havi.ui.event;
public class HTextEvent extends java.awt.AWTEvent {
    public static final int TEXT_FIRST = 2019, TEXT_START_CHANGE = 2019, TEXT_CHANGE = 2020, TEXT_END_CHANGE = 2021, TEXT_CARET_CHANGE = 2022,
        CARET_NEXT_CHAR = 2023, CARET_NEXT_LINE = 2024, CARET_NEXT_PAGE = 2025, CARET_PREV_CHAR = 2026, CARET_PREV_LINE = 2027, CARET_PREV_PAGE = 2028, TEXT_LAST = 2028;
    public HTextEvent(org.havi.ui.HKeyboardInputPreferred s, int id) { super(s, id); }
}
