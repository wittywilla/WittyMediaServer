package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.net.Address
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

// The same cases every client must pass.
class AddressTest {
    @Test
    fun sharedCases() {
        val file = File("../../shared/data/connection-cases.json")
        val cases = Json.parseToJsonElement(file.readText()).jsonObject["cases"]!!.jsonArray
        for (element in cases) {
            val case = element.jsonObject
            val input = case["input"]!!.jsonPrimitive.content
            val expected = case["url"]?.jsonPrimitive?.content?.let { Address.Result.Ok(it) }
                ?: Address.Result.Bad(case["error"]!!.jsonPrimitive.content)
            assertEquals("address \"$input\"", expected, Address.normalize(input))
        }
    }
}
