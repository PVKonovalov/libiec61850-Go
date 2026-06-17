/*
 *  common.go
 *
 *  Copyright 2014-2024 Michael Zillgith
 *  Copyright 2026 Pavel Konovalov Golang port
 *
 *  This file is part of libIEC61850.
 *
 *  libIEC61850 is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by
 *  the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  libIEC61850 is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU General Public License for more details.
 *
 *  You should have received a copy of the GNU General Public License
 *  along with libIEC61850.  If not, see <http://www.gnu.org/licenses/>.
 *
 *  See COPYING file for the complete license text.
 */

// Package common defines IEC 61850 standard types shared by both client
// and server sides of the communication stack.
//
// Key concepts:
//   - Functional Constraint (FC): categorizes data attributes (e.g., ST=status, MX=measurands)
//   - Quality: validity and source information for measured values
//   - Timestamp: IEC 61850 UTC timestamp
//   - Control models: direct/SBO with normal/enhanced security
package common
