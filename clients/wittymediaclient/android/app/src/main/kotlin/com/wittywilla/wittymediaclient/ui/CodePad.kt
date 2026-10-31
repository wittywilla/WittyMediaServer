package com.wittywilla.wittymediaclient.ui

// The number pad's rules, apart from its screen, so they can be tested without one.

const val CODE_LENGTH = 6

data class CodeEntry(val digits: String = "", val busy: Boolean = false) {
    val complete: Boolean get() = digits.length == CODE_LENGTH

    fun typed(digit: Char): CodeEntry =
        if (busy || complete || digit !in '0'..'9') this else copy(digits = digits + digit)

    fun erased(): CodeEntry = if (busy) this else copy(digits = digits.dropLast(1))

    // What's on screen: how many have been typed, not which.
    val shown: String get() = "•".repeat(digits.length) + "–".repeat(CODE_LENGTH - digits.length)
}

enum class PadMove { Up, Down, Left, Right }

const val PAD_DELETE = 9
const val PAD_ZERO = 10

// The keys in reading order. The 0 is twice as wide, so every column has something
// below it and pressing down from any digit does what it looks like it should.
val PAD_KEYS = listOf("1", "2", "3", "4", "5", "6", "7", "8", "9", "delete", "0")
private val PAD_ROWS = listOf(listOf(0, 1, 2), listOf(3, 4, 5), listOf(6, 7, 8), listOf(PAD_DELETE, PAD_ZERO, PAD_ZERO))

// Where an arrow goes from a key, or null when it leaves the pad.
fun padMove(key: Int, move: PadMove): Int? {
    val row = PAD_ROWS.indexOfFirst { key in it }
    if (row < 0) return null
    val column = PAD_ROWS[row].indexOf(key)
    return when (move) {
        PadMove.Up -> PAD_ROWS.getOrNull(row - 1)?.get(column)
        PadMove.Down -> PAD_ROWS.getOrNull(row + 1)?.get(column)
        PadMove.Left -> PAD_ROWS[row].getOrNull(column - 1)
        PadMove.Right -> PAD_ROWS[row].getOrNull(PAD_ROWS[row].lastIndexOf(key) + 1)
    }
}
