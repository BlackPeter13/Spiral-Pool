"""Proof test: _safe_num() handles every input type miner APIs can return.

This test proves that wrapping .get() calls with _safe_num() is a no-op
for already-correct numeric values, and a crash-preventer for string-encoded
values (confirmed in Innosilicon, observed in stock Antminer CGMiner).

Run: python -m pytest tests/test_safe_num.py -v
"""
import sys
import os

# Add dashboard source to path so we can import _safe_num directly
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src', 'dashboard'))

from dashboard import _safe_num


class TestSafeNumPassthrough:
    """Prove _safe_num is a no-op for values that already work."""

    def test_int_zero(self):
        """Most common case: .get() default of 0"""
        assert _safe_num(0) == 0
        assert type(_safe_num(0)) is int  # preserves int type

    def test_positive_int(self):
        """Normal share counts, fan RPMs, uptime seconds"""
        assert _safe_num(1500) == 1500
        assert _safe_num(86400) == 86400

    def test_negative_int(self):
        """Some temp sensors report negative (exhaust in cold environments)"""
        assert _safe_num(-5) == -5

    def test_positive_float(self):
        """Hashrates, temperatures, voltages"""
        assert _safe_num(85.5) == 85.5
        assert _safe_num(0.001) == 0.001
        assert _safe_num(13500.0) == 13500.0

    def test_float_zero(self):
        assert _safe_num(0.0) == 0.0


class TestSafeNumStringConversion:
    """Prove _safe_num fixes the actual crash scenario: string-encoded numbers."""

    def test_string_int(self):
        """Innosilicon returns '1500' instead of 1500 for fan RPM"""
        assert _safe_num("1500") == 1500.0

    def test_string_float(self):
        """Innosilicon returns '85.5' instead of 85.5 for temperature"""
        assert _safe_num("85.5") == 85.5

    def test_string_zero(self):
        """Some APIs return '0' instead of 0"""
        assert _safe_num("0") == 0.0

    def test_string_negative(self):
        """Edge case: negative string"""
        assert _safe_num("-12") == -12.0

    def test_string_with_spaces(self):
        """Some APIs pad with spaces"""
        assert _safe_num(" 1500 ") == 1500.0

    def test_string_scientific(self):
        """Unlikely but possible: scientific notation string"""
        assert _safe_num("1.5e3") == 1500.0


class TestSafeNumBadInput:
    """Prove _safe_num gracefully handles garbage without crashing."""

    def test_none(self):
        """dict.get() returns None when key missing and no default"""
        assert _safe_num(None) == 0

    def test_empty_string(self):
        """Some APIs return '' for missing values"""
        assert _safe_num("") == 0

    def test_non_numeric_string(self):
        """Garbage string won't crash"""
        assert _safe_num("N/A") == 0
        assert _safe_num("error") == 0

    def test_bool_true(self):
        """Python bool is subclass of int — True=1"""
        assert _safe_num(True) == 1

    def test_bool_false(self):
        """False=0"""
        assert _safe_num(False) == 0

    def test_custom_default(self):
        """Verify custom default works"""
        assert _safe_num(None, default=-1) == -1
        assert _safe_num("bad", default=42) == 42

    def test_dict_returns_default(self):
        """If somehow a nested dict leaks through"""
        assert _safe_num({"nested": "object"}) == 0

    def test_list_returns_default(self):
        """If somehow a list leaks through"""
        assert _safe_num([1, 2, 3]) == 0


class TestSafeNumArithmetic:
    """Prove _safe_num output works in downstream arithmetic — the actual operations
    that would crash if a string got through."""

    def test_comparison_gt(self):
        """Reproduces: `if degree_c > 0` — crashes with '85.5' > 0 in Py3"""
        val = _safe_num("85.5")
        assert val > 0  # would TypeError without _safe_num

    def test_comparison_eq_zero(self):
        val = _safe_num("0")
        assert not (val > 0)

    def test_division(self):
        """Reproduces: `hashrate_ths = ghs / 1000`"""
        ghs = _safe_num("13500")
        ths = ghs / 1000
        assert ths == 13.5

    def test_multiplication(self):
        """Reproduces: `hashrate_ghs = hashrate_ths * 1000`"""
        ths = _safe_num("13.5")
        ghs = ths * 1000
        assert ghs == 13500.0

    def test_addition(self):
        """Reproduces: `rejected = _safe_num(...) + _safe_num(...)`"""
        a = _safe_num("100")
        b = _safe_num("50")
        assert a + b == 150.0

    def test_int_conversion(self):
        """Reproduces: `fan_speed = min(100, int(avg_rpm / 60))`"""
        rpm = _safe_num("3600")
        pct = min(100, int(rpm / 60))
        assert pct == 60

    def test_sum_of_list(self):
        """Reproduces: `sum(fan_speeds) / len(fan_speeds)`"""
        raw = ["1500", "1600", "1400"]
        speeds = [_safe_num(v) for v in raw]
        avg = sum(speeds) / len(speeds)
        assert avg == 1500.0

    def test_max_of_list(self):
        """Reproduces: `max(chip_temps)` — crashes if mixed str/int"""
        raw = ["85", 90, "78.5"]
        temps = [_safe_num(v) for v in raw]
        assert max(temps) == 90

    def test_truthiness_zero(self):
        """Reproduces: `if not ghs:` — 0 is falsy, '0' is truthy (BUG without _safe_num)"""
        # Without _safe_num, "0" from API is truthy → skips fallback to MHS
        # With _safe_num, 0.0 is falsy → correctly falls through
        val = _safe_num("0")
        assert not val  # 0.0 is falsy ✓

    def test_truthiness_nonzero(self):
        """Non-zero is truthy"""
        val = _safe_num("13500")
        assert val  # 13500.0 is truthy ✓


class TestSafeNumInDictGet:
    """Simulate actual .get() patterns from the miner fetch functions."""

    def test_get_with_default(self):
        """Most common pattern: data.get('key', 0)"""
        data = {"temp1": "85", "fan1": 1500, "missing_key": None}
        assert _safe_num(data.get("temp1", 0)) == 85.0
        assert _safe_num(data.get("fan1", 0)) == 1500
        assert _safe_num(data.get("nonexistent", 0)) == 0
        assert _safe_num(data.get("missing_key", 0)) == 0  # None → 0

    def test_get_nested_fallback(self):
        """Pattern: _safe_num(data.get('GHS av', 0)) or _safe_num(data.get('GHS 5s', 0))"""
        data = {"GHS av": "0", "GHS 5s": "13500"}
        ghs = _safe_num(data.get('GHS av', 0)) or _safe_num(data.get('GHS 5s', 0))
        assert ghs == 13500.0

    def test_get_nested_first_wins(self):
        """When first value is valid and nonzero"""
        data = {"GHS av": "13500", "GHS 5s": "12000"}
        ghs = _safe_num(data.get('GHS av', 0)) or _safe_num(data.get('GHS 5s', 0))
        assert ghs == 13500.0

    def test_innosilicon_string_encoded_response(self):
        """Simulate actual Innosilicon API response with string values"""
        stat = {
            "Type": "A10 Pro",
            "temp1": "75",
            "temp2": "68",
            "temp3": "72",
            "fan1": "4200",
            "fan2": "4100",
            "Power": "1350",
            "temp2_1": "55",
        }
        chip_temps = []
        for i in range(1, 4):
            temp = _safe_num(stat.get(f"temp{i}", 0))
            if temp and temp > 0:
                chip_temps.append(temp)
        assert chip_temps == [75.0, 68.0, 72.0]

        fan_speeds = []
        for i in range(1, 5):
            fan = _safe_num(stat.get(f"fan{i}", 0))
            if fan and fan > 0:
                fan_speeds.append(fan)
        assert fan_speeds == [4200.0, 4100.0]

        power = _safe_num(stat.get("Power", 0))
        assert power == 1350.0

    def test_normal_numeric_response(self):
        """Simulate normal API response (int/float) — prove no regression"""
        stat = {
            "temp1": 75, "temp2": 68, "temp3": 72,
            "fan1": 4200, "fan2": 4100,
            "Power": 1350,
            "GHS av": 500.5,
            "Accepted": 12345,
            "Elapsed": 86400,
        }
        assert _safe_num(stat["temp1"]) == 75
        assert type(_safe_num(stat["temp1"])) is int  # int preserved
        assert _safe_num(stat["GHS av"]) == 500.5
        assert type(_safe_num(stat["GHS av"])) is float  # float preserved
        assert _safe_num(stat["Accepted"]) == 12345
        assert _safe_num(stat["Elapsed"]) == 86400

    def test_goldshell_temp_string_format(self):
        """Goldshell returns temp as '77.3 °C' — _safe_num catches the numeric part
        only if pre-parsed. The Goldshell function already strips °C before calling."""
        # After stripping in fetch_goldshell: temp_val = "77.3"
        assert _safe_num("77.3") == 77.3
        # Raw "77.3 °C" would fail gracefully
        assert _safe_num("77.3 °C") == 0  # falls back to default — handled upstream


class TestSafeNumEdgeCasesForExistingPatterns:
    """Test specific patterns from the codebase to prove no behavioral change."""

    def test_or_pattern_both_zero(self):
        """_safe_num(x) or _safe_num(y) when both are 0 → 0"""
        assert (_safe_num(0) or _safe_num(0)) == 0

    def test_or_pattern_first_nonzero(self):
        assert (_safe_num(100) or _safe_num(200)) == 100

    def test_or_pattern_first_zero(self):
        assert (_safe_num(0) or _safe_num(200)) == 200

    def test_list_comprehension_filter(self):
        """Pattern: [_safe_num(s.get(f'temp{i}', 0)) for i in range(1,4) if _safe_num(s.get(f'temp{i}', 0)) > 0]"""
        stat = {"temp1": "85", "temp2": 0, "temp3": "72"}
        result = [_safe_num(stat.get(f"temp{i}", 0)) for i in range(1, 4) if _safe_num(stat.get(f"temp{i}", 0)) > 0]
        assert result == [85.0, 72.0]

    def test_efficiency_division_by_zero_safe(self):
        """Pattern: power_watts / hashrate_ths if hashrate_ths > 0 else 0"""
        power_watts = _safe_num("1350")
        hashrate_ths = _safe_num("0")
        efficiency = power_watts / hashrate_ths if hashrate_ths > 0 else 0
        assert efficiency == 0  # no ZeroDivisionError

    def test_min_100_pattern(self):
        """Pattern: min(100, int(sum(speeds) / len(speeds) / 60))"""
        speeds = [_safe_num("6000"), _safe_num("5800")]
        pct = min(100, int(sum(speeds) / len(speeds) / 60))
        assert pct == 98

    def test_str_conversion_for_best_diff(self):
        """Pattern: str(summary_data.get('Best Share', 0)) — str() of _safe_num not needed here
        but verify _safe_num result converts to str fine"""
        val = _safe_num(123456)
        assert str(val) == "123456"
        val2 = _safe_num("123456")
        assert str(val2) == "123456.0"  # float str representation — this goes into best_diff
