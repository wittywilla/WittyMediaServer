package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.ui.CODE_LENGTH
import com.wittywilla.wittymediaclient.ui.CodeEntry
import com.wittywilla.wittymediaclient.ui.PAD_DELETE
import com.wittywilla.wittymediaclient.ui.PAD_KEYS
import com.wittywilla.wittymediaclient.ui.PAD_ZERO
import com.wittywilla.wittymediaclient.ui.PadMove
import com.wittywilla.wittymediaclient.ui.padMove
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class CodePadTest {
    private fun typed(text: String) = text.fold(CodeEntry()) { entry, digit -> entry.typed(digit) }

    @Test
    fun sixDigitsCompleteIt() {
        assertFalse(typed("12345").complete)
        val full = typed("123456")
        assertTrue(full.complete)
        assertEquals("123456", full.digits)
    }

    @Test
    fun aSeventhDigitIsIgnored() {
        assertEquals("123456", typed("1234567").digits)
    }

    @Test
    fun onlyDigitsCount() {
        assertEquals("12", typed("1a-2 ").digits)
    }

    @Test
    fun deleteRemovesTheLast() {
        assertEquals("12", typed("123").erased().digits)
        assertEquals("", CodeEntry().erased().digits)
    }

    @Test
    fun nothingChangesWhileSending() {
        val sending = typed("123456").copy(busy = true)
        assertEquals(sending, sending.typed('7'))
        assertEquals(sending, sending.erased())
    }

    @Test
    fun theScreenShowsHowManyNotWhich() {
        assertEquals("••––––", typed("42").shown)
        assertEquals("––––––", CodeEntry().shown)
        assertEquals(CODE_LENGTH, typed("123456").shown.length)
    }

    private fun key(label: String) = PAD_KEYS.indexOf(label)

    private fun walk(from: String, vararg moves: PadMove): String? {
        var at: Int? = key(from)
        for (m in moves) at = at?.let { padMove(it, m) }
        return at?.let { PAD_KEYS[it] }
    }

    @Test
    fun arrowsMoveAroundTheGrid() {
        assertEquals("2", walk("1", PadMove.Right))
        assertEquals("4", walk("1", PadMove.Down))
        assertEquals("5", walk("1", PadMove.Right, PadMove.Down))
        assertEquals("1", walk("5", PadMove.Up, PadMove.Left))
    }

    @Test
    fun everyDigitInTheBottomRowCanReachTheBottomRow() {
        assertEquals("delete", walk("7", PadMove.Down))
        assertEquals("0", walk("8", PadMove.Down))
        assertEquals("0", walk("9", PadMove.Down))
        assertEquals("8", walk("0", PadMove.Up))
        assertEquals("7", walk("delete", PadMove.Up))
        assertEquals("0", walk("delete", PadMove.Right))
        assertEquals("delete", walk("0", PadMove.Left))
    }

    @Test
    fun edgesLeaveThePad() {
        assertNull(padMove(key("1"), PadMove.Up))
        assertNull(padMove(key("1"), PadMove.Left))
        assertNull(padMove(key("3"), PadMove.Right))
        assertNull(padMove(key("0"), PadMove.Right))
        assertNull(padMove(key("0"), PadMove.Down))
        assertNull(padMove(PAD_DELETE, PadMove.Down))
        assertNull(padMove(PAD_DELETE, PadMove.Left))
    }

    @Test
    fun everyKeyCanBeReachedWithOnlyArrows() {
        val reached = mutableSetOf(0)
        val waiting = ArrayDeque(listOf(0))
        while (waiting.isNotEmpty()) {
            val at = waiting.removeFirst()
            for (move in PadMove.entries) {
                val next = padMove(at, move) ?: continue
                if (reached.add(next)) waiting.add(next)
            }
        }
        assertEquals(PAD_KEYS.indices.toSet(), reached)
        assertNotNull(PAD_KEYS[PAD_ZERO])
    }
}
